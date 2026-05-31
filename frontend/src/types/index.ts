export interface User {
  id: string;
  email: string;
  name: string;
  isAdmin: boolean;
  isTeamAdmin: boolean;
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
  bggWatermarkId?: string;
  members: User[];
  createdAt: string;
  updatedAt: string;
}

export interface TeamSettings {
  bggWatermarkId?: string;
  bggCoverOffsetX?: number;
  bggCoverOffsetY?: number;
  episodeNewsUrl?: string;
  hasEpisodeNewsBearerToken?: boolean;
}

export interface Watermark {
  id: string;
  userId: string;
  teamId?: string;
  name: string;
  filename: string;
  url: string;
  createdAt: string;
}

export interface BGGGameData {
  gameId: string;
  title: string;
  yearPublished?: string;
  minPlayers: string;
  maxPlayers: string;
  minPlaytime: string;
  maxPlaytime: string;
  minAge: string;
  designers: string[];
  artists: string[];
  publishers: string[];
  rating?: string;
  weight?: string;
  imageBase64: string;
  imageFilename: string;
  suggestedContent: string;
}

export type PostStatus = 'draft' | 'scheduled' | 'published' | 'failed';
export type PostType = 'post' | 'story' | 'reel';
export type Platform = 'bluesky' | 'instagram' | 'twitter' | 'mastodon' | 'threads' | 'linkedin';

export interface PostResult {
  platform: Platform;
  success: boolean;
  postId?: string;
  error?: string;
  postedAt?: string;
}

export interface NewsResult {
  success: boolean;
  error?: string;
  sentAt?: string;
}

export interface EpisodeNews {
  enabled: boolean;
  episodeNumber?: string;
  title?: string;
  additionalText?: string;
  bggLink?: string;
  result?: NewsResult;
}

export interface Post {
  id: string;
  userId: string;
  postType?: PostType;
  content: string;
  firstComment?: string;
  imageUrls?: string[];
  platforms: Platform[];
  scheduledAt: string;
  status: PostStatus;
  results?: PostResult[];
  tags?: string[];
  accountIds?: Record<string, string>;
  suffixIds?: Record<string, string>;
  contentOverrides?: Record<string, string>;
  episodeNews?: EpisodeNews;
  createdAt: string;
  updatedAt: string;
}

export interface Suffix {
  id: string;
  userId: string;
  name: string;
  content: string;
  createdAt: string;
  updatedAt: string;
}

export interface MentionEntry {
  id: string;
  userId: string;
  teamId?: string;
  name: string;
  handles: Record<string, string>; // platform -> @handle
  createdAt: string;
  updatedAt: string;
}

export interface SocialAccount {
  id: string;
  teamId?: string;
  platform: Platform;
  accountName: string;
  displayName: string;
  avatarUrl?: string;
  isActive: boolean;
  tokenExpiry?: string;
  did?: string;
  pdsHost?: string;
  igUserId?: string;
  twitterUserId?: string;
  mastodonInstance?: string;
  threadsUserId?: string;
  linkedinPersonUrn?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AppSettings {
  id?: string;
  appUrl: string;
  instagramAppId: string;
  webhookVerifyToken: string;
  adobeExpressClientId: string;
  allowSelfRegistration: boolean;
  imprintHtml: string;
  cookieBannerEnabled: boolean;
  cookieBannerText: string;
  openRouterModel: string;
  aiLanguage: string;
  hasOpenRouterKey?: boolean;
  hasBggApiToken?: boolean;
  updatedAt?: string;
}

export interface PublicSettings {
  adobeExpressClientId: string;
  imprintHtml: string;
  cookieBannerEnabled: boolean;
  cookieBannerText: string;
  openRouterEnabled: boolean;
  hasBggApiToken: boolean;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export type ConventionQueueItemStatus = 'pending' | 'approved' | 'scheduled' | 'published';

export interface ConventionQueue {
  id: string;
  userId: string;
  teamId?: string;
  name: string;
  conventionUrl?: string;
  hashtags?: string[];
  startDate: string;
  endDate: string;
  postsPerDay: number;
  timeSlots: string[];
  platforms: Platform[];
  accountIds?: Record<string, string>;
  suffixIds?: Record<string, string>;
  status: string;
  itemCount?: number;
  approvedCount?: number;
  scheduledCount?: number;
  createdAt: string;
  updatedAt: string;
}

export interface ConventionQueueItem {
  id: string;
  queueId: string;
  postId?: string;
  imageUrl: string;
  bggUrl?: string;
  caption?: string;
  status: ConventionQueueItemStatus;
  aiError?: string;
  sortOrder: number;
  platforms?: Platform[];
  accountIds?: Record<string, string>;
  suffixIds?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface SchedulePreviewSlot {
  itemId: string;
  scheduledAt: string;
}

export interface SchedulePreview {
  slots: SchedulePreviewSlot[];
  approvedCount: number;
  availableSlots: number;
  overflow: number;
}
