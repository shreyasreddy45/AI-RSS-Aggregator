import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TEST_NAME = __ENV.TEST_NAME || 'k6 load test user';
const TEST_EMAIL = __ENV.TEST_EMAIL || 'k6-load-user@example.com';
const TEST_PASSWORD = __ENV.TEST_PASSWORD || 'LoadTestPass123!';
const SEARCH_QUERIES = [
  'AI news',
  'rss aggregator',
  'postgres performance',
  'machine learning',
  'startups funding',
];

const JSON_HEADERS = {
  'Content-Type': 'application/json',
};

export function setup() {
  const credentials = {
    name: TEST_NAME,
    email: TEST_EMAIL,
    password: TEST_PASSWORD,
  };

  const createRes = http.post(
    `${BASE_URL}/v1/users`,
    JSON.stringify(credentials),
    { headers: JSON_HEADERS }
  );

  if (createRes.status !== 201 && createRes.status !== 500) {
    throw new Error(`Failed to create test user: ${createRes.status} ${createRes.body}`);
  }

  const loginRes = http.post(
    `${BASE_URL}/v1/login`,
    JSON.stringify({ email: TEST_EMAIL, password: TEST_PASSWORD }),
    { headers: JSON_HEADERS }
  );

  check(loginRes, {
    'login succeeded': (r) => r.status === 200,
  });

  if (loginRes.status !== 200) {
    throw new Error(`Failed to log in test user: ${loginRes.status} ${loginRes.body}`);
  }

  return { token: loginRes.json('token') };
}

export const options = {
  scenarios: {
    mixed_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 50 },
        { duration: '120s', target: 50 },
        { duration: '30s', target: 0 },
        { duration: '30s', target: 100 },
        { duration: '120s', target: 100 },
        { duration: '30s', target: 0 },
      ],
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'http_req_duration{scenario:mixed_load}': ['p(95)<500', 'p(99)<1000'],
    'http_req_failed{scenario:mixed_load}': ['rate<0.01'],
    'checks{scenario:mixed_load}': ['rate>0.99'],
  },
};

function authHeaders(token) {
  return {
    ...JSON_HEADERS,
    Authorization: `Bearer ${token}`,
  };
}

function getPosts(token) {
  const res = http.get(`${BASE_URL}/v1/posts?limit=10&offset=0`, {
    headers: authHeaders(token),
  });

  check(res, {
    'GET /v1/posts succeeded': (r) => r.status === 200,
  });
}

function getSearch(token) {
  const query = SEARCH_QUERIES[Math.floor(Math.random() * SEARCH_QUERIES.length)];
  const res = http.get(`${BASE_URL}/v1/search?q=${encodeURIComponent(query)}&limit=10&offset=0`, {
    headers: authHeaders(token),
  });

  check(res, {
    'GET /v1/search succeeded': (r) => r.status === 200,
  });
}

function getRecommendations(token) {
  const res = http.get(`${BASE_URL}/v1/recommendations?limit=10&offset=0`, {
    headers: authHeaders(token),
  });

  check(res, {
    'GET /v1/recommendations succeeded': (r) => r.status === 200,
  });
}

function feedFollowMutation(token) {
  const feedsRes = http.get(`${BASE_URL}/v1/feeds`, {
    headers: authHeaders(token),
  });

  if (feedsRes.status !== 200 || !feedsRes.json() || feedsRes.json().length === 0) {
    return;
  }

  const feed = feedsRes.json()[0];

  const createRes = http.post(
    `${BASE_URL}/v1/feedfollows`,
    JSON.stringify({ feed_id: feed.id }),
    { headers: authHeaders(token) }
  );

  check(createRes, {
    'POST /v1/feedfollows successful or duplicate': (r) => r.status === 201 || r.status === 500,
  });

  if (createRes.status !== 201) {
    return;
  }

  const followId = createRes.json('id');
  if (!followId) {
    return;
  }

  const deleteRes = http.del(`${BASE_URL}/v1/feedfollows/${followId}`, null, {
    headers: authHeaders(token),
  });

  check(deleteRes, {
    'DELETE /v1/feedfollows/{id} succeeded': (r) => r.status === 200 || r.status === 404,
  });
}

function agentChat(token) {
  const payload = {
    query: 'Summarize the most relevant article content and tell me which sources are most useful for answering this question.',
  };

  const res = http.post(`${BASE_URL}/v1/agent/chat`, JSON.stringify(payload), {
    headers: authHeaders(token),
  });

  check(res, {
    'POST /v1/agent/chat responded': (r) => r.status === 200 || r.status === 500 || r.status === 503,
  });
}

export default function (data) {
  const token = data.token;
  const bucket = Math.random() * 100;

  if (bucket < 40) {
    getPosts(token);
  } else if (bucket < 65) {
    getSearch(token);
  } else if (bucket < 85) {
    getRecommendations(token);
  } else if (bucket < 95) {
    feedFollowMutation(token);
  } else {
    agentChat(token);
  }

  sleep(Math.random() * 0.2 + 0.1);
}
