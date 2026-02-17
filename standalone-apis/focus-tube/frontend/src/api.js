const API_BASE = import.meta.env.VITE_API_BASE || "";

export function getToken() {
  return localStorage.getItem("token");
}

export function setToken(token) {
  localStorage.setItem("token", token);
}

export function setEmail(email) {
  localStorage.setItem("user_email", email);
}

export function getEmail() {
  return localStorage.getItem("user_email") || "";
}

export function clearToken() {
  localStorage.removeItem("token");
  localStorage.removeItem("user_email");
}

export function isLoggedIn() {
  return !!getToken();
}

function authHeaders() {
  const token = getToken();
  return {
    Authorization: token ? `Bearer ${token}` : "",
    "Content-Type": "application/json",
  };
}

// Callback set by App to handle logout on 401
let onUnauthorized = null;
export function setOnUnauthorized(cb) {
  onUnauthorized = cb;
}

async function request(url, options = {}) {
  const res = await fetch(API_BASE + url, { headers: authHeaders(), ...options });
  if (res.status === 401) {
    clearToken();
    if (onUnauthorized) onUnauthorized();
    throw new Error("Session expired");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const err = new Error(body.error || res.statusText);
    err.status = res.status;
    throw err;
  }
  return res.json();
}

export async function requestCode(email) {
  const res = await fetch(API_BASE + "/auth/request-code", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  });
  return res.json();
}

export async function verifyCode(email, code) {
  const res = await fetch(API_BASE + "/auth/verify-code", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, code }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || "Verification failed");
  }
  return res.json();
}

export async function logout() {
  const token = getToken();
  if (token) {
    await fetch(API_BASE + "/auth/logout", {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    }).catch(() => {});
  }
  clearToken();
}

export async function listRecords(bucket, opts = {}) {
  const params = new URLSearchParams();
  if (opts.limit) params.set("limit", opts.limit);
  if (opts.offset) params.set("offset", opts.offset);
  if (opts.orderBy) params.set("order_by", opts.orderBy);
  if (opts.orderDir) params.set("order_dir", opts.orderDir);
  const qs = params.toString();
  const url = `/buckets/mine/${bucket}${qs ? `?${qs}` : ""}`;
  const res = await request(url);
  if (opts.limit || opts.offset) {
    return { data: res.data || [], total: res.total || 0 };
  }
  return res.data || [];
}

export async function createRecord(bucket, data) {
  return request(`/buckets/mine/${bucket}`, {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function updateRecord(bucket, id, data) {
  return request(`/buckets/mine/${bucket}/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteRecord(bucket, id) {
  return request(`/buckets/mine/${bucket}/${id}`, { method: "DELETE" });
}

export async function batchRecords(bucket, records, { dedupeOn } = {}) {
  const qs = dedupeOn ? `?dedupe_on=${encodeURIComponent(dedupeOn)}` : "";
  return request(`/buckets/mine/${bucket}/batch${qs}`, {
    method: "POST",
    body: JSON.stringify(records),
  });
}

export async function resolveChannel(url) {
  return request(`/api/resolve-channel?url=${encodeURIComponent(url)}`);
}

export async function fetchRSS(channelId) {
  return request(`/api/fetch-rss?channel_id=${encodeURIComponent(channelId)}`);
}

export async function fetchAllVideos(channelId) {
  return request(`/api/fetch-all-videos?channel_id=${encodeURIComponent(channelId)}`);
}
