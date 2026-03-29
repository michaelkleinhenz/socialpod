const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

class ApiClient {
  private token: string | null = null;

  setToken(token: string | null) {
    this.token = token;
    if (token) {
      localStorage.setItem('token', token);
    } else {
      localStorage.removeItem('token');
    }
  }

  getToken(): string | null {
    if (!this.token) {
      this.token = localStorage.getItem('token');
    }
    return this.token;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      ...(options.headers as Record<string, string> || {}),
    };

    if (this.getToken()) {
      headers['Authorization'] = `Bearer ${this.getToken()}`;
    }

    if (!(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }

    const res = await fetch(`${API_URL}/api${path}`, {
      ...options,
      headers,
    });

    if (res.status === 401) {
      this.setToken(null);
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Request failed' }));
      throw new Error(err.error || 'Request failed');
    }

    return res.json();
  }

  // Auth
  login(email: string, password: string) {
    return this.request<{ token: string; user: any }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
  }

  register(email: string, password: string, name: string) {
    return this.request<{ token: string; user: any }>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password, name }),
    });
  }

  getMe() {
    return this.request<any>('/auth/me');
  }

  generateApiToken() {
    return this.request<{ apiToken: string }>('/auth/api-token', { method: 'POST' });
  }

  // Posts
  getPosts(params?: { start?: string; end?: string; status?: string; platform?: string }) {
    const query = new URLSearchParams();
    if (params?.start) query.set('start', params.start);
    if (params?.end) query.set('end', params.end);
    if (params?.status) query.set('status', params.status);
    if (params?.platform) query.set('platform', params.platform);
    const qs = query.toString();
    return this.request<any[]>(`/posts${qs ? '?' + qs : ''}`);
  }

  createPost(data: any) {
    return this.request<any>('/posts', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  updatePost(id: string, data: any) {
    return this.request<any>(`/posts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  deletePost(id: string) {
    return this.request<any>(`/posts/${id}`, { method: 'DELETE' });
  }

  reschedulePost(id: string, scheduledAt: string) {
    return this.request<any>(`/posts/${id}/reschedule`, {
      method: 'PATCH',
      body: JSON.stringify({ scheduledAt }),
    });
  }

  uploadImage(file: File) {
    const form = new FormData();
    form.append('image', file);
    return this.request<{ url: string; filename: string }>('/upload', {
      method: 'POST',
      body: form,
    });
  }

  // Admin
  getAccounts() {
    return this.request<any[]>('/admin/accounts');
  }

  addBlueskyAccount(data: { handle: string; appPassword: string; pdsHost?: string }) {
    return this.request<any>('/admin/accounts/bluesky', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  deleteAccount(id: string) {
    return this.request<any>(`/admin/accounts/${id}`, { method: 'DELETE' });
  }

  toggleAccount(id: string) {
    return this.request<any>(`/admin/accounts/${id}/toggle`, { method: 'PATCH' });
  }

  getSettings() {
    return this.request<any>('/admin/settings');
  }

  updateSettings(data: any) {
    return this.request<any>('/admin/settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  getInstagramAuthUrl() {
    return this.request<{ url: string }>('/admin/instagram/auth-url');
  }

  getUsers() {
    return this.request<any[]>('/admin/users');
  }

  deleteUser(id: string) {
    return this.request<any>(`/admin/users/${id}`, { method: 'DELETE' });
  }
}

export const api = new ApiClient();
