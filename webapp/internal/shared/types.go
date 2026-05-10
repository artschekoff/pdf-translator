package shared

type OcrSettings struct {
	Engine               string   `json:"engine"`
	Lang                 []string `json:"lang"`
	DoOcr                bool     `json:"do_ocr"`
	DoTableStructure     bool     `json:"do_table_structure"`
	TableMode            string   `json:"table_mode"`
	GeneratePictureImages bool    `json:"generate_picture_images"`
	ImagesScale          float64  `json:"images_scale"`
	DocumentTimeout      float64  `json:"document_timeout"`
}

type BlockData struct {
	ID    string                 `json:"id"`
	Type  string                 `json:"type"`
	Data  map[string]any `json:"data"`
	Order int                    `json:"order"`
}
