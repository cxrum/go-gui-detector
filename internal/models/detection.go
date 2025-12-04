package models

type DetectionResult struct {
	Label      string    `json:"label"`
	Confidence float32   `json:"confidence"`
	Box        []float32 `json:"box"`
}

type Box struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}
