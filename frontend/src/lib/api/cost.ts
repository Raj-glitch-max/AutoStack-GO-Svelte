import type { CostEstimateResponse } from "$lib/types/cost";

async function fetchFromAPI<T>(endpoint: string, options?: RequestInit): Promise<T | undefined> {
    let data: T | undefined;

    const token = localStorage.getItem("pocketbase_auth");
    if (!token) {
        return data;
    }
    const authHeader = { Authorization: `Bearer ${JSON.parse(token).token}` };

    const baseUrl = window.location.hostname === "localhost" ? `http://localhost:8090` : "";
    const url = `${baseUrl}/api/cost/${endpoint}`;

    try {
        const response = await fetch(url, {
            ...options,
            headers: {
                ...authHeader,
                ...options?.headers
            }
        });
        
        if (!response.ok) {
            throw new Error(`API request failed: ${response.statusText}`);
        }
        
        data = (await response.json()) as T;
    } catch (error) {
        console.error("Cost API error:", error);
        throw error;
    }

    return data;
}

export async function getCostEstimate(
    blueprint: string,
    region: string,
    variables?: Record<string, any>
): Promise<CostEstimateResponse | undefined> {
    const params = new URLSearchParams({
        blueprint,
        region
    });
    
    const endpoint = `estimate?${params.toString()}`;
    
    if (variables && Object.keys(variables).length > 0) {
        return fetchFromAPI<CostEstimateResponse>(endpoint, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(variables)
        });
    }
    
    return fetchFromAPI<CostEstimateResponse>(endpoint);
}
