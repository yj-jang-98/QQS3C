package engineconfig

import (
	"encoding/json"
	"os"
)

type Config struct {
	Backend     string      `json:"backend"`
	DT          float64     `json:"dt"`
	N           int         `json:"N"`
	A           [][]float64 `json:"A"`
	B           [][]float64 `json:"B"`
	C           [][]float64 `json:"C"`
	L           [][]float64 `json:"L"`
	QDiag       []float64   `json:"QDiag"`
	RDiag       []float64   `json:"RDiag"`
	QfScale     float64     `json:"QfScale"`
	AlphaMax    float64     `json:"alphaMax"`
	UMax        float64     `json:"uMax"`
	Sigma0      float64     `json:"sigma0"`
	LambdaParam float64     `json:"lambda"`
	K           int         `json:"K"`
	ChebOrder   int         `json:"chebOrder"`
	ChebBound   float64     `json:"chebBound"`
	ChebEta     float64     `json:"chebEta"`
	ChebClip    bool        `json:"chebClip"`
	X0          []float64   `json:"x0"`
}

func Load(path string) (Config, bool) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, false
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, false
	}
	return cfg, true
}
