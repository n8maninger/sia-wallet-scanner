# Sia Wallet Scanner

Command line tool to scan a seed for address usage. All used addresses are output
to a CSV file

## Usage

1. Download and unzip release
2. Open terminal
3. Run `./scanner`
4. Enter your seed words with one space between
5. Press enter
6. Output will show in the terminal 
	+ "Used" is the total number of addresses
	+ "Last" is the index of the last used address
	+ "Gap" is the difference between the last used address and the current scan index
7. Wait for the scanner to exit
8. Open addresses.csv

### Optional CLI Flags

+ `--lookahead` sets the maximum **gap** between unused addresses before the scanner completes. A higher number will search more addresses and take longer, a lower number may not find all addresses if there are large gaps. Defaults to `100000`.
+ `--start` sets the start index for the search, should not be changed. Defaults to `0`. 

## Building 
1. Clone Repo
2. Run `make static`
3. The binary is in `bin/`