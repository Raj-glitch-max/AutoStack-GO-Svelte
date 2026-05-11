/**
 * AutoStack Load Test — Cost Estimation API
 * 
 * Tests: <500ms response time under 1000 concurrent requests
 * Tool: k6 (https://k6.io)
 * 
 * Run: k6 run scripts/load-test-cost-api.js
 * With env: k6 run -e BASE_URL=http://localhost:8090 -e AUTH_TOKEN=your_token scripts/load-test-cost-api.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const responseTime = new Trend('response_time_ms');

export const options = {
  scenarios: {
    // Ramp up to 1000 concurrent users
    cost_estimation_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 100 },   // Ramp to 100 users
        { duration: '60s', target: 500 },   // Ramp to 500 users
        { duration: '60s', target: 1000 },  // Ramp to 1000 users
        { duration: '60s', target: 1000 },  // Hold at 1000 users
        { duration: '30s', target: 0 },     // Ramp down
      ],
    },
  },
  thresholds: {
    // 99% of requests must complete within 500ms
    'http_req_duration': ['p(99)<500'],
    // Error rate must stay below 1%
    'errors': ['rate<0.01'],
    // Custom metric: response time
    'response_time_ms': ['p(95)<500'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8090';
const AUTH_TOKEN = __ENV.AUTH_TOKEN || '';

const blueprints = ['ecs-web-app', 'full-stack', 'static-site'];
const regions = ['us-east-1', 'us-west-2', 'eu-west-1'];

export default function () {
  const blueprint = blueprints[Math.floor(Math.random() * blueprints.length)];
  const region = regions[Math.floor(Math.random() * regions.length)];

  const payload = JSON.stringify({ blueprint, region });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': AUTH_TOKEN,
    },
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/api/aws/cost-estimate`, payload, params);
  const duration = Date.now() - start;

  responseTime.add(duration);

  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 500ms': (r) => r.timings.duration < 500,
    'has estimate field': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.estimate !== undefined || body.total !== undefined;
      } catch {
        return false;
      }
    },
  });

  errorRate.add(!success);
  sleep(0.1); // 100ms think time
}

export function handleSummary(data) {
  return {
    'scripts/load-test-results-cost-api.json': JSON.stringify(data, null, 2),
    stdout: `
=== Cost Estimation API Load Test Results ===
Total Requests:    ${data.metrics.http_reqs.values.count}
Failed Requests:   ${data.metrics.http_req_failed.values.passes}
Error Rate:        ${(data.metrics.errors?.values.rate * 100 || 0).toFixed(2)}%
Avg Response Time: ${data.metrics.http_req_duration.values.avg.toFixed(0)}ms
P95 Response Time: ${data.metrics.http_req_duration.values['p(95)'].toFixed(0)}ms
P99 Response Time: ${data.metrics.http_req_duration.values['p(99)'].toFixed(0)}ms
Max Response Time: ${data.metrics.http_req_duration.values.max.toFixed(0)}ms

PASS: ${data.metrics.http_req_duration.values['p(99)'] < 500 ? '✅ P99 < 500ms' : '❌ P99 >= 500ms'}
`,
  };
}
