package toncenter

type Nft struct {
	Address string `json:"address"`
	Content struct {
		Domain string `json:"domain"`
	} `json:"content"`
}
