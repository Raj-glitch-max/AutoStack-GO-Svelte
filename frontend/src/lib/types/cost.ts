export interface CostRange {
    min: number;
    max: number;
}

export interface CostBreakdown {
    compute: number;
    networking: number;
    storage: number;
    transfer: number;
}

export interface CostEstimate {
    total: number;
    range: CostRange;
    breakdown: CostBreakdown;
    assumptions: Record<string, string>;
    disclaimer: string;
    pricingFetchedAt: string;
}

export interface CostEstimateResponse {
    estimate: CostEstimate;
}
