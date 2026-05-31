const API_URL = import.meta.env.VITE_API_URL || '';

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

  createPost(data: any, files?: File[]) {
    const form = new FormData();
    form.append('data', JSON.stringify(data));
    files?.forEach(f => form.append('images', f));
    return this.request<any>('/posts', { method: 'POST', body: form });
  }

  updatePost(id: string, data: any, files?: File[]) {
    const form = new FormData();
    form.append('data', JSON.stringify(data));
    files?.forEach(f => form.append('images', f));
    return this.request<any>(`/posts/${id}`, { method: 'PUT', body: form });
  }

  deletePost(id: string) {
    return this.request<any>(`/posts/${id}`, { method: 'DELETE' });
  }

  retryPost(id: string) {
    return this.request<any>(`/posts/${id}/retry`, { method: 'POST' });
  }

  retryEpisodeNews(id: string) {
    return this.request<any>(`/posts/${id}/retry-news`, { method: 'POST' });
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

  uploadFromURL(url: string) {
    return this.request<{ url: string; filename: string }>('/upload-from-url', {
      method: 'POST',
      body: JSON.stringify({ url }),
    });
  }

  // Active accounts (available to all authenticated users, for preview)
  getActiveAccounts() {
    return this.request<any[]>('/accounts');
  }

  // Admin
  getAccounts() {
    return this.request<any[]>('/admin/accounts');
  }

  addBlueskyAccount(data: { handle: string; appPassword: string; pdsHost?: string; teamId?: string }) {
    return this.request<any>('/admin/accounts/bluesky', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addTwitterAccount(data: { consumerKey: string; consumerSecret: string; accessToken: string; accessTokenSecret: string; teamId?: string }) {
    return this.request<any>('/admin/accounts/twitter', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addMastodonAccount(data: { instance: string; accessToken: string; teamId?: string }) {
    return this.request<any>('/admin/accounts/mastodon', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addThreadsAccount(data: { accessToken: string; threadsUserId?: string; teamId?: string }) {
    return this.request<any>('/admin/accounts/threads', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addLinkedInAccount(data: { accessToken: string; linkedinPersonUrn?: string; teamId?: string }) {
    return this.request<any>('/admin/accounts/linkedin', {
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

  assignAccountTeam(id: string, teamId: string | null) {
    return this.request<any>(`/admin/accounts/${id}/team`, {
      method: 'PATCH',
      body: JSON.stringify({ teamId }),
    });
  }

  // Team admin endpoints
  getTeamAccounts() {
    return this.request<any[]>('/team/accounts');
  }

  addTeamBlueskyAccount(data: { handle: string; appPassword: string; pdsHost?: string }) {
    return this.request<any>('/team/accounts/bluesky', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addTeamTwitterAccount(data: { consumerKey: string; consumerSecret: string; accessToken: string; accessTokenSecret: string }) {
    return this.request<any>('/team/accounts/twitter', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addTeamMastodonAccount(data: { instance: string; accessToken: string }) {
    return this.request<any>('/team/accounts/mastodon', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addTeamThreadsAccount(data: { accessToken: string; threadsUserId?: string }) {
    return this.request<any>('/team/accounts/threads', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  addTeamLinkedInAccount(data: { accessToken: string; linkedinPersonUrn?: string }) {
    return this.request<any>('/team/accounts/linkedin', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  deleteTeamAccount(id: string) {
    return this.request<any>(`/team/accounts/${id}`, { method: 'DELETE' });
  }

  toggleTeamAccount(id: string) {
    return this.request<any>(`/team/accounts/${id}/toggle`, { method: 'PATCH' });
  }

  getTeamInstagramAuthUrl() {
    return this.request<{ url: string }>('/team/instagram/auth-url');
  }

  getTeamMembers() {
    return this.request<any[]>('/team/members');
  }

  addTeamMember(data: { email: string }) {
    return this.request<any>('/team/members', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  removeTeamMember(id: string) {
    return this.request<any>(`/team/members/${id}`, { method: 'DELETE' });
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

  getInstagramAuthUrl(teamId?: string) {
    const qs = teamId ? `?teamId=${teamId}` : '';
    return this.request<{ url: string }>(`/admin/instagram/auth-url${qs}`);
  }

  getLinkedInAuthUrl(teamId?: string) {
    const qs = teamId ? `?teamId=${teamId}` : '';
    return this.request<{ url: string }>(`/admin/linkedin/auth-url${qs}`);
  }

  getYouTubeAuthUrl(teamId?: string) {
    const qs = teamId ? `?teamId=${teamId}` : '';
    return this.request<{ url: string }>(`/admin/youtube/auth-url${qs}`);
  }

  getTeamLinkedInAuthUrl() {
    return this.request<{ url: string }>('/team/linkedin/auth-url');
  }

  getTeamYouTubeAuthUrl() {
    return this.request<{ url: string }>('/team/youtube/auth-url');
  }

  getMastodonAuthUrl(instance: string) {
    return this.request<{ url: string; redirectUri: string }>(`/admin/mastodon/auth-url?instance=${encodeURIComponent(instance)}`);
  }

  getTeamMastodonAuthUrl(instance: string) {
    return this.request<{ url: string; redirectUri: string }>(`/team/mastodon/auth-url?instance=${encodeURIComponent(instance)}`);
  }

  getUsers() {
    return this.request<any[]>('/admin/users');
  }

  createUser(data: { email: string; password: string; name: string; isAdmin: boolean }) {
    return this.request<any>('/admin/users', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  deleteUser(id: string) {
    return this.request<any>(`/admin/users/${id}`, { method: 'DELETE' });
  }

  updateUserRole(id: string, data: { isAdmin?: boolean; isTeamAdmin?: boolean }) {
    return this.request<any>(`/admin/users/${id}/role`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    });
  }

  adminResetUserPassword(id: string) {
    return this.request<{ password: string }>(`/admin/users/${id}/reset-password`, { method: 'POST' });
  }

  updatePassword(currentPassword: string, newPassword: string) {
    return this.request<{ message: string }>('/auth/password', {
      method: 'PUT',
      body: JSON.stringify({ currentPassword, newPassword }),
    });
  }

  getRegistrationStatus() {
    return this.request<{ allowed: boolean; firstUser: boolean }>('/auth/registration-status');
  }

  getPublicSettings() {
    return this.request<import('../types').PublicSettings>('/settings/public');
  }

  // AI text generation
  generateText(prompt: string, platforms: string[]) {
    return this.request<{ text: string }>('/generate-text', {
      method: 'POST',
      body: JSON.stringify({ prompt, platforms }),
    });
  }

  // Dashboard stats (no AI)
  getDashboardStats() {
    return this.request<any>('/dashboard/stats');
  }

  // Dashboard AI insights
  getDashboardInsights() {
    return this.request<{
      stats: { label: string; value: string; trend: 'up' | 'down' | 'neutral' }[];
      recommendations: { title: string; description: string; priority: 'high' | 'medium' | 'low' }[];
    }>('/dashboard/ai-insights', { method: 'POST' });
  }

  // Suffixes
  getSuffixes() {
    return this.request<any[]>('/suffixes');
  }

  createSuffix(data: { name: string; content: string }) {
    return this.request<any>('/suffixes', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  updateSuffix(id: string, data: { name?: string; content?: string }) {
    return this.request<any>(`/suffixes/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  deleteSuffix(id: string) {
    return this.request<any>(`/suffixes/${id}`, { method: 'DELETE' });
  }

  // Mentions
  getMentions() {
    return this.request<any[]>('/mentions');
  }

  createMention(data: { name: string; handles: Record<string, string> }) {
    return this.request<any>('/mentions', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  updateMention(id: string, data: { name?: string; handles?: Record<string, string> }) {
    return this.request<any>(`/mentions/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  deleteMention(id: string) {
    return this.request<any>(`/mentions/${id}`, { method: 'DELETE' });
  }

  // Account feed (Instagram or Bluesky)
  getFeed(accountId?: string, platform?: string) {
    const query = new URLSearchParams();
    if (accountId) query.set('accountId', accountId);
    if (platform) query.set('platform', platform);
    const qs = query.toString();
    return this.request<any[]>(`/inbox/feed${qs ? '?' + qs : ''}`);
  }

  // Watermarks
  getWatermarks() {
    return this.request<any[]>('/watermarks');
  }

  uploadWatermark(name: string, file: File) {
    const form = new FormData();
    form.append('name', name);
    form.append('image', file);
    return this.request<any>('/watermarks', { method: 'POST', body: form });
  }

  deleteWatermark(id: string) {
    return this.request<any>(`/watermarks/${id}`, { method: 'DELETE' });
  }

  // Teams
  getTeams() {
    return this.request<any[]>('/admin/teams');
  }

  createTeam(data: { name: string }) {
    return this.request<any>('/admin/teams', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  deleteTeam(id: string) {
    return this.request<any>(`/admin/teams/${id}`, { method: 'DELETE' });
  }

  setTeamMembers(id: string, userIds: string[]) {
    return this.request<any>(`/admin/teams/${id}/members`, {
      method: 'PUT',
      body: JSON.stringify({ userIds }),
    });
  }

  generateTeamToken(id: string) {
    return this.request<{ apiToken: string }>(`/admin/teams/${id}/token`, { method: 'POST' });
  }

  createOwnTeam(data: { name: string }) {
    return this.request<any>('/team/setup', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  // Convention mode
  getConventionQueues() {
    return this.request<any[]>('/convention/queues');
  }

  createConventionQueue(data: any) {
    return this.request<any>('/convention/queues', { method: 'POST', body: JSON.stringify(data) });
  }

  getConventionQueue(id: string) {
    return this.request<{ queue: any; items: any[] }>(`/convention/queues/${id}`);
  }

  updateConventionQueue(id: string, data: any) {
    return this.request<any>(`/convention/queues/${id}`, { method: 'PUT', body: JSON.stringify(data) });
  }

  deleteConventionQueue(id: string) {
    return this.request<any>(`/convention/queues/${id}`, { method: 'DELETE' });
  }

  addConventionItem(queueId: string, file: File) {
    const form = new FormData();
    form.append('image', file);
    return this.request<any>(`/convention/queues/${queueId}/items`, { method: 'POST', body: form });
  }

  addBGGItems(queueId: string, urls: string[]) {
    return this.request<{ items: any[]; errors: { url: string; error: string }[] }>(
      `/convention/queues/${queueId}/items/bgg`,
      { method: 'POST', body: JSON.stringify({ urls }) },
    );
  }

  updateConventionItem(queueId: string, itemId: string, data: any) {
    return this.request<any>(`/convention/queues/${queueId}/items/${itemId}`, { method: 'PUT', body: JSON.stringify(data) });
  }

  replaceConventionItemImage(queueId: string, itemId: string, file: File) {
    const form = new FormData();
    form.append('image', file);
    return this.request<any>(`/convention/queues/${queueId}/items/${itemId}/image`, { method: 'PUT', body: form });
  }

  deleteConventionItem(queueId: string, itemId: string) {
    return this.request<any>(`/convention/queues/${queueId}/items/${itemId}`, { method: 'DELETE' });
  }

  analyzeConventionItem(queueId: string, itemId: string) {
    return this.request<any>(`/convention/queues/${queueId}/items/${itemId}/analyze`, { method: 'POST' });
  }

  analyzeAllConventionItems(queueId: string) {
    return this.request<any>(`/convention/queues/${queueId}/analyze-all`, { method: 'POST' });
  }

  reorderConventionItems(queueId: string, itemIds: string[]) {
    return this.request<any>(`/convention/queues/${queueId}/reorder`, { method: 'POST', body: JSON.stringify({ itemIds }) });
  }

  previewConventionSchedule(queueId: string) {
    return this.request<any>(`/convention/queues/${queueId}/preview`);
  }

  scheduleConventionItems(queueId: string) {
    return this.request<any>(`/convention/queues/${queueId}/schedule`, { method: 'POST' });
  }

  // BGG integration
  fetchBGGGame(url: string) {
    return this.request<import('../types').BGGGameData>(`/bgg/fetch?url=${encodeURIComponent(url)}`);
  }

  // Team settings (BGG watermark)
  getTeamSettings() {
    return this.request<import('../types').TeamSettings>('/team/settings');
  }

  updateTeamSettings(data: { bggWatermarkId?: string | null; bggCoverOffsetX?: number; bggCoverOffsetY?: number; episodeNewsUrl?: string; episodeNewsBearerToken?: string }) {
    return this.request<{ message: string }>('/team/settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  // Admin BGG settings (manage any team's BGG overlay config)
  getAdminTeamBggSettings(teamId: string) {
    return this.request<import('../types').TeamSettings>(`/admin/teams/${teamId}/settings`);
  }

  updateAdminTeamBggSettings(teamId: string, data: { bggWatermarkId?: string | null; bggCoverOffsetX?: number; bggCoverOffsetY?: number; episodeNewsUrl?: string; episodeNewsBearerToken?: string }) {
    return this.request<{ message: string }>(`/admin/teams/${teamId}/settings`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }
}

export const api = new ApiClient();
