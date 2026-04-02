package cost

// CostEstimateWithRange represents a cost estimate with min/max range
type CostEstimateWithRange struct {
	Estimate  float64     `json:"estimate"`
	RangeMin  float64     `json:"rangeMin"`
	RangeMax  float64     `json:"rangeMax"`
	Breakdown interface{} `json:"breakdown"`
	Currency  string      `json:"currency"`
}

// ServiceCostBreakdown represents a generic service cost breakdown
type ServiceCostBreakdown struct {
	ServiceName   string             `json:"serviceName"`
	TotalMonthly  float64            `json:"totalMonthly"`
	Components    map[string]float64 `json:"components"`
	Currency      string             `json:"currency"`
	Assumptions   map[string]string  `json:"assumptions"`
}

// CostCalculator is the interface all service calculators must implement
type CostCalculator interface {
	Calculate(config interface{}) (interface{}, error)
	CalculateWithRange(config interface{}) (*CostEstimateWithRange, error)
	EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error)
}
