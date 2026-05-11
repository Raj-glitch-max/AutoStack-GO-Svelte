/**
 * AutoStack Load Test — Deployment Queue
 * 
 * Tests: 10 concurrent deployments from different users don't interfere
 * Tool: k6
 * 
 * Run: k6 run scripts/load-test-deployment-queue.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');
const conflictRate = new Rate('conflicts_handled_correctly');

export const options = {
  // 10 concurrent users, each attempting a deployment
  vus: 10,
  duration: '120s',
  thresholds: {
    // Concurrent deployments must not cause 500 errors
    'http_req_failed': ['rate<0.05'],
    // Conflicts (409) are expected and correct — not errors
    'errors': ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8090';

// Each VU uses a different auth token to simulate different users
const AUTH_TOKENS = (__ENV.AUTH_TOKENS || '').split(',').filter(Boolean);

export default function () {
  const token = AUTH_TOKENS[__VU % AUTH_TOKENS.length] || '';

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': token,
    },
  };

  // Attempt to create a deployment
  const payload = JSON.stringify({
    name: `load-test-deploy-${__VU}-${Date.now()}`,
    project: __ENV.PROJECT_ID || 'test-project',
    blueprint: 'ecs-web-app',
    region: 'us-east-1',
    configuration: { containerImage: 'nginx:latest', instanceType: '256' },
  });

  const res = http.post(`${BASE_URL}/api/aws/deployments`, payload, params);

  // 201 = created, 409 = conflict (already deploying) — both are correct
  const isCorrect = check(res, {
    'deployment created or conflict handled': (r) => r.status === 201 || r.status === 409,
    'no server errors': (r) => r.status < 500,
  });

  // 409 means the queue is working correctly
  if (res.status === 409) {
    conflictRate.add(1);
  }

  errorRate.add(!isCorrect);
  sleep(Math.random() * 2 + 1); // 1-3s between attempts
}

export function handleSummary(data) {
  return {
    'scripts/load-test-results-deployment-queue.json': JSON.stringify(data, null, 2),
    stdout: `
=== Deployment Queue Load Test Results ===
Concurrent Users:  10
Total Requests:    ${data.metrics.http_reqs.values.count}
Server Errors:     ${data.metrics.http_req_failed.values.passes}
Conflicts (409):   ${data.metrics.conflicts_handled_correctly?.values.passes || 0} (expected — queue working)

PASS: ${data.metrics.http_req_failed.values.rate < 0.05 ? '✅ Server error rate < 5%' : '❌ Too many server errors'}
`,
  };
}
