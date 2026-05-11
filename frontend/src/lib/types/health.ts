export type HealthStatus = 'healthy' | 'unhealthy' | 'unknown' | 'pending' | 'degraded';

export interface ResourceHealth {
    resourceId: string;
    resourceType: string;
    status: HealthStatus;
    message: string;
    details?: any;
}

export interface DeploymentHealth {
    deploymentId: string;
    deploymentType: 'aws' | 'k8s';
    overallStatus: HealthStatus;
    resources: ResourceHealth[];
}
