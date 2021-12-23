package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/n8maninger/sia-wallet-scanner/api"
	mnemonics "gitlab.com/NebulousLabs/entropy-mnemonics"
	siacrypto "go.sia.tech/siad/crypto"
	"go.sia.tech/siad/types"
)

var englishWordMap = func() map[string]bool {
	m := make(map[string]bool, len(mnemonics.EnglishDictionary))
	for _, v := range mnemonics.EnglishDictionary {
		m[v] = true
	}
	return m
}()

func convertRecoveryPhrase(phrase string) (seed [32]byte, _ error) {
	for _, char := range phrase {
		if unicode.IsUpper(char) {
			return [32]byte{}, errors.New("seed is not valid: all words must be lowercase")
		}

		if !unicode.IsLetter(char) && !unicode.IsSpace(char) {
			return [32]byte{}, fmt.Errorf("seed is not valid: illegal character '%v'", char)
		}
	}

	// Check seed has 28 or 29 words
	words := strings.Fields(strings.TrimSpace(phrase))
	if len(words) != 28 && len(words) != 29 {
		return [32]byte{}, errors.New("seed is not valid: must be 28 or 29 words")
	}

	for _, word := range words {
		if _, ok := englishWordMap[word]; !ok {
			return [32]byte{}, fmt.Errorf("unrecognized word %q in seed phrase", word)
		}
	}
	// Decode the string into the checksummed byte slice.
	checksumSeedBytes, err := mnemonics.FromString(strings.Join(words, " "), mnemonics.DictionaryID("english"))
	if err != nil {
		return [32]byte{}, fmt.Errorf("unable to decode mnemonic: %w", err)
	}

	if len(checksumSeedBytes) != 38 {
		return [32]byte{}, fmt.Errorf("seed is not valid: wrong number of bytes %d expected 38", len(checksumSeedBytes))
	}

	copy(seed[:], checksumSeedBytes)

	checksum := siacrypto.HashObject(seed)
	if len(checksumSeedBytes) != siacrypto.EntropySize+6 || !bytes.Equal(checksum[:6], checksumSeedBytes[siacrypto.EntropySize:]) {
		return [32]byte{}, fmt.Errorf("unable to validate seed: incorrect checksum: usually a flipped or missing word")
	}
	return
}

func generateAddress(seed [32]byte, index uint64) types.UnlockHash {
	_, pk := siacrypto.GenerateKeyPairDeterministic(siacrypto.HashAll(seed, index))
	return types.UnlockConditions{
		PublicKeys:         []types.SiaPublicKey{types.Ed25519PublicKey(pk)},
		SignaturesRequired: 1,
	}.UnlockHash()
}

// highestUsedIndex returns the highest index that has been seen on the
// blockchain, or 0 if none of the addresses have been seen.
func highestUsedIndex(seed [32]byte, start, end uint64) (max uint64, used []address, _ error) {
	addresses := make([]types.UnlockHash, 0, end-start)
	// maps addresses to their index relative to start.
	usage := make(map[string]uint64)
	for i := start; i < end; i++ {
		addr := generateAddress(seed, i)
		addresses = append(addresses, addr)
		usage[addr.String()] = i
	}

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	requestBody := map[string]interface{}{
		"addresses": addresses,
	}
	if err := enc.Encode(requestBody); err != nil {
		return 0, nil, fmt.Errorf("unable to encode request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.siacentral.com/v2/wallet/addresses/used", buf)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to send request: %w", err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var respBody api.AddressesResp
	if err := dec.Decode(&respBody); err != nil {
		return 0, nil, fmt.Errorf("unable to decode response body: %w", err)
	} else if respBody.Type != "success" {
		return 0, nil, fmt.Errorf("unable to get highest index: %q", respBody.Message)
	}

	for _, addr := range respBody.Addresses {
		if max < usage[addr.Address] {
			max = usage[addr.Address]
		}
		var uh types.UnlockHash
		if err := uh.LoadString(addr.Address); err != nil {
			return 0, nil, fmt.Errorf("unable to parse address %v: %w", addr.Address, err)
		}
		used = append(used, address{
			index: usage[addr.Address],
			addr:  uh,
		})
	}
	sort.Slice(used, func(i, j int) bool {
		return used[i].index < used[j].index
	})
	return
}

type address struct {
	index uint64
	addr  types.UnlockHash
}

func main() {
	var outputDir string
	var lookahead, start uint64
	flag.StringVar(&outputDir, "outdir", ".", "output directory")
	flag.Uint64Var(&lookahead, "lookahead", 1e5, "number of addresses to lookahead")
	flag.Uint64Var(&start, "start", 0, "the address index to start at")
	flag.Parse()

	fmt.Printf("Seed: ")
	s := bufio.NewScanner(os.Stdin)
	s.Scan()
	if err := s.Err(); err != nil {
		log.Fatalln("unable to read seed:", err)
	}

	seed, err := convertRecoveryPhrase(s.Text())
	if err != nil {
		log.Fatalln("unable to parse seed:", err)
	}

	fmt.Println("Starting Detection... Press CTRL+C to stop.")

	f, err := os.Create(filepath.Join(outputDir, "addresses.csv"))
	if err != nil {
		log.Fatalln("unable to create file:", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	var stopped bool
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		stopped = true
	}()

	var detected, gap, last uint64
	const lookup = 250
	for start, end := start, start+lookup; gap <= lookahead; start, end = start+lookup, end+lookup {
		// handle interrupt
		if stopped {
			fmt.Println("\nStopping...")
			break
		}

		max, used, err := highestUsedIndex(seed, start, end)
		if err != nil {
			log.Fatalln("unable to get highest index:", err)
		}
		if max > last {
			last = max
		}
		gap = end - last
		for _, addr := range used {
			err := w.Write([]string{
				strconv.FormatUint(addr.index, 10),
				addr.addr.String(),
			})
			if err != nil {
				log.Fatalln("unable to write address to output file:", err)
			}
		}
		detected += uint64(len(used))
		fmt.Fprintf(os.Stdout, "\x1b[1K\rChecking Address: %d-%d, Used: %d, Last: %d, Gap %d", start, end, detected, last, gap)
	}
}
