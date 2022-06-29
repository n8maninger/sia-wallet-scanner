package api

type (
	// AddressUsage is the usage of an address on the Sia blockchain.
	AddressUsage struct {
		Address   string `json:"address"`
		UsageType string `json:"usage_type"`
	}

	// AddressesResp is the response from the /v2/wallet/addresses endpoint.
	AddressesResp struct {
		Message   string         `json:"message"`
		Type      string         `json:"type"`
		Addresses []AddressUsage `json:"addresses"`
	}
)
