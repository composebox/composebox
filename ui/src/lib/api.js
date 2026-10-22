export async function getAuthStatus() {
  const res = await fetch('/api/auth/status');
  if (!res.ok) throw new Error('Failed to fetch auth status');
  return res.json();
}

export async function login(password) {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  });
  const data = await res.json();
  if (!res.ok || !data.success) {
    throw new Error(data.error || 'Invalid password');
  }
  return data;
}

export async function setup(password) {
  const res = await fetch('/api/auth/setup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  });
  const data = await res.json();
  if (!res.ok || !data.success) {
    throw new Error(data.error || 'Failed to set password');
  }
  return data;
}

export async function logout() {
  await fetch('/api/auth/logout', { method: 'POST' });
}

export async function getStacks() {
  const res = await fetch('/api/stacks');
  if (res.status === 401) {
    const err = new Error('Unauthorized');
    err.status = 401;
    throw err;
  }
  if (!res.ok) throw new Error('Failed to load stacks');
  return res.json();
}

export async function getStack(name) {
  const res = await fetch(`/api/stacks/${encodeURIComponent(name)}`);
  if (!res.ok) throw new Error('Stack not found');
  return res.json();
}

export async function saveStack(name, composeContent, envContent) {
  const res = await fetch(`/api/stacks/${encodeURIComponent(name)}/save`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      compose_content: composeContent,
      env_content: envContent,
    }),
  });
  const data = await res.json();
  if (!res.ok || !data.success) {
    throw new Error(data.error || data.message || 'Failed to save stack');
  }
  return data;
}

export async function createStack(name) {
  const res = await fetch('/api/stacks', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  const data = await res.json();
  if (!res.ok || !data.success) {
    throw new Error(data.error || data.message || 'Failed to create stack');
  }
  return data;
}

export async function deleteStack(name) {
  const res = await fetch(`/api/stacks/${encodeURIComponent(name)}`, { method: 'DELETE' });
  const data = await res.json();
  if (!res.ok || !data.success) {
    throw new Error(data.error || data.message || 'Failed to delete stack');
  }
  return data;
}

export async function triggerAction(name, action, onChunk) {
  const res = await fetch(`/api/stacks/${encodeURIComponent(name)}/${encodeURIComponent(action)}`, {
    method: 'POST',
  });
  if (!res.ok) {
    const errText = await res.text();
    throw new Error(errText || `${action} failed`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let fullOutput = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    const chunk = decoder.decode(value, { stream: true });
    fullOutput += chunk;
    if (onChunk) {
      onChunk(chunk, fullOutput);
    }
  }

  if (fullOutput.includes('[ERROR]')) {
    throw new Error(`Action '${action}' finished with error`);
  }

  return { success: true, output: fullOutput };
}
