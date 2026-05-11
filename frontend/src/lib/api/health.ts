import { client } from "$lib/pocketbase";
import type { DeploymentHealth } from "$lib/types/health";

export async function getHealthSummary(): Promise<DeploymentHealth[]> {
    try {
        const response = await fetch(`${client.baseUrl}/api/health/summary`, {
            headers: {
                'Authorization': `Bearer ${client.authStore.token}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Failed to fetch health summary');
        }
        
        return await response.json();
    } catch (error) {
        console.error('Health summary fetch error:', error);
        return [];
    }
}

export async function getHealthDetail(deploymentId: string): Promise<DeploymentHealth | null> {
    try {
        const response = await fetch(`${client.baseUrl}/api/health/detail/${deploymentId}`, {
            headers: {
                'Authorization': `Bearer ${client.authStore.token}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Failed to fetch health detail');
        }
        
        return await response.json();
    } catch (error) {
        console.error('Health detail fetch error:', error);
        return null;
    }
}
