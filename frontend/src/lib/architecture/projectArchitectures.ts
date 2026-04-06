export interface ArchitectureNode {
  id: string;
  label: string;
  icon: string;
  x: number;
  y: number;
  delay: number;
}

export interface ArchitectureConnection {
  from: string;
  to: string;
  delay: number;
  label?: string;
}

export interface StatusUpdate {
  delay: number;
  message: string;
  status: string;
  color: string;
}

export interface ArchitectureConfig {
  nodes: ArchitectureNode[];
  connections: ArchitectureConnection[];
  terminalCommands: string[];
  statusUpdates: StatusUpdate[];
  duration: number;
}

export const PROJECT_ARCHITECTURES: Record<string, ArchitectureConfig> = {
  'AutoStack': {
    nodes: [
      // Frontend Layer (Top Row) - Full width spacing
      { id: 'user', label: 'Users', icon: 'user', x: 200, y: 150, delay: 0 },
      { id: 'svelte', label: 'SvelteKit', icon: 'svelte', x: 960, y: 150, delay: 0.5 },
      { id: 'tailwind', label: 'Tailwind', icon: 'css', x: 1720, y: 150, delay: 0.8 },
      
      // Load Balancer Layer - Wide spread
      { id: 'nginx', label: 'Nginx', icon: 'nginx', x: 480, y: 320, delay: 1.0 },
      { id: 'alb', label: 'AWS ALB', icon: 'aws', x: 1440, y: 320, delay: 1.2 },
      
      // Backend Services Layer - Generous spacing
      { id: 'go-api', label: 'Go API', icon: 'go', x: 320, y: 490, delay: 1.5 },
      { id: 'websocket', label: 'WebSocket', icon: 'monitor', x: 960, y: 490, delay: 1.8 },
      { id: 'pocketbase', label: 'PocketBase', icon: 'database', x: 1600, y: 490, delay: 2.0 },
      
      // Infrastructure Layer - Maximum spread
      { id: 'terraform', label: 'Terraform', icon: 'terraform', x: 240, y: 660, delay: 2.0 },
      { id: 'credentials', label: 'Credentials', icon: 'security', x: 720, y: 660, delay: 2.2 },
      { id: 'k8s-master', label: 'K8s Master', icon: 'kubernetes', x: 1200, y: 660, delay: 2.5 },
      { id: 'pricing', label: 'Cost Track', icon: 'dollar', x: 1680, y: 660, delay: 3.2 },
      
      // Deployment Layer - Full width utilization
      { id: 'k8s-nodes', label: 'K8s Nodes', icon: 'server', x: 400, y: 830, delay: 2.7 },
      { id: 'ecs', label: 'ECS Fargate', icon: 'aws', x: 960, y: 830, delay: 2.5 },
      { id: 'rds', label: 'RDS', icon: 'database', x: 1520, y: 830, delay: 2.7 },
      
      // Storage & Monitoring Layer - Bottom spread
      { id: 's3', label: 'S3 Storage', icon: 'storage', x: 560, y: 1000, delay: 3.0 },
      { id: 'cloudwatch', label: 'CloudWatch', icon: 'monitor', x: 1360, y: 1000, delay: 3.0 }
    ],
    
    connections: [
      // Frontend Flow - Wide connections
      { from: 'user', to: 'svelte', delay: 0.6, label: 'Web Interface' },
      { from: 'svelte', to: 'tailwind', delay: 0.9, label: 'Styling' },
      
      // Load Balancer Routing - Spread out
      { from: 'svelte', to: 'nginx', delay: 1.1, label: 'K8s Traffic' },
      { from: 'svelte', to: 'alb', delay: 1.3, label: 'AWS Traffic' },
      
      // Backend Services - Clean wide paths
      { from: 'nginx', to: 'go-api', delay: 1.6, label: 'API Calls' },
      { from: 'alb', to: 'websocket', delay: 1.9, label: 'WebSocket' },
      { from: 'go-api', to: 'pocketbase', delay: 2.1, label: 'Database' },
      
      // Infrastructure Management - Spacious connections
      { from: 'websocket', to: 'credentials', delay: 2.3, label: 'Security' },
      { from: 'terraform', to: 'k8s-master', delay: 2.6, label: 'K8s Deploy' },
      { from: 'terraform', to: 'ecs', delay: 2.6, label: 'AWS Deploy' },
      { from: 'credentials', to: 'pricing', delay: 2.4, label: 'Cost API' },
      
      // Deployment Layer - Wide spread
      { from: 'k8s-master', to: 'k8s-nodes', delay: 2.8, label: 'Scheduling' },
      { from: 'ecs', to: 'rds', delay: 2.8, label: 'Database' },
      
      // Storage & Monitoring - Bottom wide layer
      { from: 'terraform', to: 's3', delay: 3.1, label: 'State Storage' },
      { from: 'go-api', to: 'cloudwatch', delay: 3.1, label: 'Monitoring' }
    ],
    
    terminalCommands: [
      'Initializing AutoStack deployment...',
      'Building SvelteKit + Tailwind frontend...',
      'Starting Go API and WebSocket services...',
      'Connecting PocketBase database...',
      'Initializing Terraform engine...',
      'Validating AWS credentials...',
      'Setting up Kubernetes cluster...',
      'Provisioning ECS and RDS...',
      'Configuring S3 and CloudWatch...',
      'Enabling cost tracking...',
      'Running health checks...',
      'AutoStack deployment complete!'
    ],
    
    statusUpdates: [
      { delay: 0.5, message: 'Frontend build started', status: 'Building', color: 'blue' },
      { delay: 1.0, message: 'Load balancers ready', status: 'Ready', color: 'green' },
      { delay: 1.5, message: 'API server online', status: 'Running', color: 'green' },
      { delay: 2.0, message: 'Database connected', status: 'Connected', color: 'green' },
      { delay: 2.5, message: 'K8s cluster ready', status: 'Deployed', color: 'green' },
      { delay: 2.7, message: 'AWS services active', status: 'Active', color: 'green' },
      { delay: 3.0, message: 'Monitoring enabled', status: 'Monitoring', color: 'green' },
      { delay: 3.5, message: 'All systems operational', status: 'Complete', color: 'green' }
    ],
    
    duration: 4.5
  }
};