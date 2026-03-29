export interface User {
  id: string;
  email: string;
  name: string;
  isAdmin: boolean;
  teamId?: string;
  teamName?: string;
  apiToken?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Team {
  id: string;
  name: string;
  apiToken?: string;
  members: User[];
  createdAt: string;
  updatedAt: string;
}

export type PostStatus = 'draft' | 'scheduled' | 'published' | 'failed';
export type Platform = 'bluesky' | 'instagram';

export interface PostResult {
  platform: Platform;
  success: boolean;
  postId?: string;
  error?: string;
  postedAt?: string;
}

export interface Post {
  id: string;
  userId: string;
  content: string;
  imageUrls?: string[];
  platforms: Platform[];
  scheduledAt: string;
  status: PostStatus;
  results?: PostResult[];
  tags?: string[];
  accountIds?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface SocialAccount {
  id: string;
  platform: Platform;
  accountName: string;
  displayName: string;
  isActive: boolean;
  tokenExpiry?: string;
  did?: string;
  pdsHost?: string;
  igUserId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AppSettings {
  id?: string;
  appUrl: string;
  instagramAppId: string;
  webhookVerifyToken: string;
  adobeExpressClientId: string;
  defaultPostTime: string;
  autoPublish: boolean;
  allowSelfRegistration: boolean;
  updatedAt?: string;
}

export interface AuthResponse {
  token: string;
  user: User;
}
