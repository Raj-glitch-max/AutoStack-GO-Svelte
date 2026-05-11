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

export interface CostAlert {
    id: string;
    deploymentId: string;
    deployment?: any; // Expanded deployment object
    expand?: any; // Expanded relations
    type: 'THRESHOLD' | 'SPIKE' | 'ANOMALY' | 'DROP' | 'anomaly_detected' | 'budget_exceeded';
    severity: 'low' | 'medium' | 'high' | 'critical';
    message: string;
    currentCost: number;
    actualCost?: number;
    estimatedCost?: number;
    variance?: number;
    threshold?: number;
    serviceBreakdown?: Record<string, number>;
    acknowledged: boolean;
    sentAt?: string;
    created?: string;
    createdAt: string;
    updatedAt: string;
    aiExplanation?: string; // AI-generated explanation for cost anomalies
}
