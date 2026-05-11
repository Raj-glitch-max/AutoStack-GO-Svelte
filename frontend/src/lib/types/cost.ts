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

export interface CostPeriod {
    start: string; // YYYY-MM-DD format
    end: string;   // YYYY-MM-DD format
}

export interface ActualCostData {
    costToDate: number;
    projectedMonthly: number;
    variance: number;
    breakdown: Record<string, number>;
    period: CostPeriod;
    fetchedAt: string;
}

export interface ActualCostEstimate {
    total: number;
    createdAt: string;
}

export interface ActualCostResponse {
    actual: ActualCostData;
    estimate: ActualCostEstimate;
}

export interface ActualCostDelayResponse {
    message: string;
    availableIn: string;
    deploymentAge: string;
}
