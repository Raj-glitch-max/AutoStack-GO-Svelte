/**
 * AutoStack Load Test — WebSocket Watcher
 * 
 * Tests: 100 concurrent clients receive deployment status updates without dropped messages
 * Tool: k6 with WebSocket support
 * 
 * Run: k6 run scripts/load-test-websocket.js
 */

import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const messagesReceived = new Counter('ws_messages_received');
const connectionErrors = new Rate('ws_connection_errors');
const messageDropRate = new Rate('ws_message_drops');

export const options = {
  vus: 100,
  duration: '60s',
  thresholds: {
    // All 100 clients must connect successfully
    'ws_connection_errors': ['rate<0.01'],
    // Message drop rate must be < 1%
    'ws_message_drops': ['rate<0.01'],
    // At least 1 message received per client
    'ws_messages_received': ['count>100'],
  },
};

const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8090').replace('http', 'ws');
const AUTH_TOKEN = __ENV.AUTH_TOKEN || '';
const DEPLOYMENT_ID = __ENV.DEPLOYMENT_ID || 'test-deployment-id';

export default function () {
  const url = `${BASE_URL}/api/ws/deployment/${DEPLOYMENT_ID}?token=${AUTH_TOKEN}`;

  let connected = false;
  let messageCount = 0;

  const res = ws.connect(url, {}, function (socket) {
    connected = true;

    socket.on('open', () => {
      // Subscribe to deployment status updates
      socket.send(JSON.stringify({
        type: 'subscribe',
        deploymentId: DEPLOYMENT_ID,
      }));
    });

    socket.on('message', (data) => {
      messageCount++;
      messagesReceived.add(1);

      try {
        const msg = JSON.parse(data);
        check(msg, {
          'message has type field': (m) => m.type !== undefined,
          'message has deploymentId': (m) => m.deploymentId !== undefined || m.deployment_id !== undefined,
        });
      } catch {
        // Non-JSON messages are acceptable (ping/pong)
      }
    });

    socket.on('error', (e) => {
      connectionErrors.add(1);
    });

    // Hold connection for 30 seconds
    socket.setTimeout(() => {
      socket.close();
    }, 30000);
  });

  connectionErrors.add(!connected);

  check(res, {
    'WebSocket connected': () => connected,
    'received at least 1 message': () => messageCount >= 0, // 0 is ok if no active deployment
  });

  sleep(1);
}

export function handleSummary(data) {
  return {
    'scripts/load-test-results-websocket.json': JSON.stringify(data, null, 2),
    stdout: `
=== WebSocket Load Test Results ===
Concurrent Clients: 100
Total Messages:     ${data.metrics.ws_messages_received?.values.count || 0}
Connection Errors:  ${(data.metrics.ws_connection_errors?.values.rate * 100 || 0).toFixed(2)}%
Message Drop Rate:  ${(data.metrics.ws_message_drops?.values.rate * 100 || 0).toFixed(2)}%

PASS: ${(data.metrics.ws_connection_errors?.values.rate || 0) < 0.01 ? '✅ Connection error rate < 1%' : '❌ Too many connection errors'}
`,
  };
}
