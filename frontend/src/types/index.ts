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
  bggHandleLookupEnabled?: boolean;
  enabledPlugins?: string[];
  newsCreatorUrl?: string;
  hasNewsCreatorBearerToken?: boolean;
  newsCreatorWatermarkId?: string;
  episodeCreatorUrl?: string;
  hasEpisodeCreatorBearerToken?: boolean;
  episodeCreatorWatermarkId?: string;
  episodeOverlayNewsId?: string;
  episodeOverlayNewsOffsetX?: number;
  episodeOverlayNewsOffsetY?: number;
  episodeOverlayReviewId?: string;
  episodeOverlayReviewOffsetX?: number;
  episodeOverlayReviewOffsetY?: number;
  episodeOverlaySpecialId?: string;
  episodeOverlaySpecialOffsetX?: number;
  episodeOverlaySpecialOffsetY?: number;
  episodeOverlayTatortId?: string;
  episodeOverlayTatortOffsetX?: number;
  episodeOverlayTatortOffsetY?: number;
  scenePromptTemplate?: string;
}

export interface Plugin {
  id: string;
  label: string;
  description: string;
}

export const AVAILABLE_PLUGINS: Plugin[] = [
  {
    id: 'episode_news',
    label: 'Episode News',
    description: 'Adds an "Add News" section to the post creation dialog, allowing team members to send episode metadata to a configured webhook when creating a post.',
  },
  {
    id: 'news_creator',
    label: 'News Creator',
    description: 'Adds a "News" page for creating news episodes and optionally scheduling social media posts.',
  },
  {
    id: 'episode_creator',
    label: 'Episode Creator',
    description: 'Adds an "Episodes" page for creating podcast episodes and optionally scheduling social media posts.',
  },
];

/** A watermark is a media library image marked as one (MediaItem.watermark). */
export type Watermark = MediaItem;

/** An entry of the team's media library (uploaded material for post images). */
/** The kinds the media library's list can be narrowed to. */
export type MediaKind = 'image' | 'video' | 'audio' | 'font' | 'color' | 'template' | 'watermark';

// Clipart (GET /api/clipart/search). From Iconify: an SVG body and its view
// box; recolorable icons draw in currentColor. From OpenClipart (once an
// admin has stored a token): a preview and the file to import.
export interface ClipartIcon {
  id: string;
  source: 'iconify' | 'openclipart';
  set: string;
  name: string;
  body?: string;
  left?: number;
  top?: number;
  width?: number;
  height?: number;
  recolorable?: boolean;
  previewUrl?: string;
  fileUrl?: string;
}

export interface ClipartSearchResult {
  icons: ClipartIcon[];
  sets: Record<string, { name: string; author?: string; license: string }>;
  // The sources that failed while others answered.
  errors?: Partial<Record<'iconify' | 'openclipart', string>>;
}

export interface MediaItem {
  id: string;
  userId: string;
  teamId?: string;
  /** Empty for a colour entry, which carries color instead of a file. */
  url: string;
  /** #rrggbb or #rrggbbaa, colour entries only. */
  color?: string;
  /** An image editor design as JSON (EditorTemplate), template entries only; url is its preview. */
  template?: string;
  /** Images only: offered as an overlay by the cropper, the image editor, convention queues and the team settings. */
  watermark?: boolean;
  name: string;
  description?: string;
  tags?: string[];
  contentType?: string;
  width?: number;
  height?: number;
  size?: number;
  createdAt: string;
  updatedAt: string;
}

/** Image editor work saved to be finished later; the user's own. */
export interface ImageDraft {
  id: string;
  userId: string;
  teamId?: string;
  name: string;
  /** A rendered preview. */
  url: string;
  /** The edit as JSON (EditorDraft); only GET /image-drafts/:id carries it. */
  design?: string;
  width?: number;
  height?: number;
  createdAt: string;
  updatedAt: string;
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
  categories: string[];
  mechanics: string[];
  rating?: string;
  weight?: string;
  imageBase64: string;
  imageFilename: string;
  description?: string;
  aiAbstract?: string;
  suggestedContent: string;
  suggestedContentByPlatform?: Record<string, string>;
  suggestedHashtags?: string[];
  publisherInstagramHandles?: string[];
}

export interface PublisherHandle {
  id: string;
  name: string;
  handles: Record<string, string>; // platform → @handle
  createdAt: string;
  updatedAt: string;
}

export type PostStatus = 'draft' | 'scheduled' | 'published' | 'failed';
export type PostType = 'post' | 'story' | 'reel';
export type Platform = 'bluesky' | 'instagram' | 'twitter' | 'mastodon' | 'threads' | 'linkedin' | 'youtube';

export interface PostInsights {
  mediaProductType?: string;
  permalink?: string;
  reach: number;
  views: number;
  shares: number;
  saved: number;
  likes: number;
  comments: number;
  totalInteractions: number;
  follows: number;
  profileVisits: number;
  fetchedAt: string;
  error?: string;
}

export interface PostResult {
  platform: Platform;
  success: boolean;
  postId?: string;
  error?: string;
  warning?: string;
  postedAt?: string;
  insights?: PostInsights;
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
  resultHistory?: NewsResult[];
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
  footerIds?: Record<string, string>;
  contentOverrides?: Record<string, string>;
  episodeNews?: EpisodeNews;
  collaborators?: string[];
  userTags?: string[];
  /** A reel's own cover image (an upload). */
  coverUrl?: string;
  /** A reel's cover frame, in ms into the video, when it has no coverUrl. */
  thumbOffset?: number;
  /** The page or BGG entry the post is about (set by MCP agents). */
  sourceUrl?: string;
  /** What created the post when not made in the editor: "mcp" or an agent's label. */
  origin?: string;
  /** The editable form of images and videos made in the image editor. */
  designs?: PostDesign[];
  createdAt: string;
  updatedAt: string;
}

/**
 * The image editor's edit of one of a post's images or videos, so the post
 * editor reopens it as it was.
 */
export interface PostDesign {
  /** The export among the post's imageUrls; absent until it is exported. */
  imageUrl?: string;
  /** A still shown in place of a design not exported yet. */
  previewUrl?: string;
  video?: boolean;
  /** The edit as JSON (EditorDraft). */
  design: string;
}

export interface NewsDraft {
  id: string;
  userId: string;
  teamId?: string;
  episodeNumber?: string;
  newsTagline?: string;
  articleUrl?: string;
  shownotes?: string;
  imageUrls?: string[];
  addSocialPosting: boolean;
  content?: string;
  platforms?: Platform[];
  scheduledAt?: string;
  status?: PostStatus;
  footerIds?: Record<string, string>;
  contentOverrides?: Record<string, string>;
  accountIds?: Record<string, string>;
  firstComment?: string;
  postType?: PostType;
  posted?: boolean;
  postedAt?: string;
  /** What created the draft when not made by hand (see Post.origin). */
  origin?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EpisodeDraft {
  id: string;
  userId: string;
  teamId?: string;
  episodeNumber?: string;
  episodeTitle?: string;
  episodeType?: string;
  summary?: string;
  episodeDate?: string;
  gameNamePublisher?: string;
  linkPublisher?: string;
  linkBGG?: string;
  rules?: string;
  scene?: string;
  introText?: string;
  imageUrls?: string[];
  addSocialPosting: boolean;
  content?: string;
  platforms?: Platform[];
  scheduledAt?: string;
  status?: PostStatus;
  footerIds?: Record<string, string>;
  contentOverrides?: Record<string, string>;
  accountIds?: Record<string, string>;
  firstComment?: string;
  postType?: PostType;
  posted?: boolean;
  postedAt?: string;
  /** What created the draft when not made by hand (see Post.origin). */
  origin?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Footer {
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
  country?: string;
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
  youtubeChannelId?: string;
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
  privacyPolicyHtml: string;
  cookieBannerEnabled: boolean;
  cookieBannerText: string;
  openRouterModel: string;
  openRouterVisionModel: string;
  aiLanguage: string;
  linkedInClientId?: string;
  hasLinkedInClientSecret?: boolean;
  youtubeClientId?: string;
  hasYouTubeClientSecret?: boolean;
  hasOpenRouterKey?: boolean;
  hasBggApiToken?: boolean;
  openClipartAppId?: string;
  hasOpenClipartApiKey?: boolean;
  hasOpenClipartAppSecret?: boolean;
  openClipartTokenExpiresAt?: string;
  mailgunBaseUrl?: string;
  mailgunDomain?: string;
  mailgunFromEmail?: string;
  hasMailgunApiKey?: boolean;
  promptGameSummary?: string;
  promptGameAbstract?: string;
  promptHashtags?: string;
  promptHandleLookup?: string;
  promptSocialPost?: string;
  promptDashboardInsights?: string;
  updatedAt?: string;
}

export interface TeamInvite {
  id: string;
  teamId: string;
  email: string;
  status: string;
  invitedBy: string;
  expiresAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface PublicSettings {
  adobeExpressClientId: string;
  imprintHtml: string;
  privacyPolicyHtml: string;
  cookieBannerEnabled: boolean;
  cookieBannerText: string;
  openRouterEnabled: boolean;
  hasBggApiToken: boolean;
  youtubeConfigured: boolean;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export type ConventionQueueItemStatus = 'pending' | 'approved' | 'scheduled';

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
  /** Minimum delay between two consecutive posts, in hours. Caps posts/day. */
  minHoursBetweenPosts?: number;
  /** Deprecated: posts are now scattered randomly instead of pinned to slots. */
  timeSlots?: string[];
  platforms: Platform[];
  accountIds?: Record<string, string>;
  footerIds?: Record<string, string>;
  /** Optional overlay watermark applied to every image at post time. */
  watermarkId?: string;
  /** Media library video template the Quick Reel puts a clip into. */
  reelTemplateId?: string;
  status: string;
  /** Time the next random post from this queue is due (set by the auto-poster). */
  nextPostAt?: string;
  itemCount?: number;
  approvedCount?: number;
  scheduledCount?: number;
  /** Running total of items from this queue that have been published. */
  postedCount?: number;
  createdAt: string;
  updatedAt: string;
}

export interface ConventionQueueItem {
  id: string;
  queueId: string;
  postId?: string;
  imageUrl: string;
  /** All images for a gallery item (a single post with multiple images). */
  imageUrls?: string[];
  bggUrl?: string;
  /** Made from gallery photos: deleting it puts its images back into the gallery. */
  fromGallery?: boolean;
  caption?: string;
  /** A separate text per platform; a platform without one gets the caption. */
  contentOverrides?: Record<string, string>;
  status: ConventionQueueItemStatus;
  aiError?: string;
  sortOrder: number;
  platforms?: Platform[];
  accountIds?: Record<string, string>;
  footerIds?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

/** A photo in a convention queue's gallery, not yet a single post or part of a carousel. */
export interface ConventionGalleryPhoto {
  id: string;
  queueId: string;
  imageUrl: string;
  createdAt: string;
}

export interface SchedulePreviewSlot {
  scheduledAt: string;
}

export interface SchedulePreview {
  /** Projected upcoming post times; a random approved photo goes out at each. */
  slots: SchedulePreviewSlot[];
  approvedCount: number;
  /** Effective posts per day after the minimum-delay cap. */
  postsPerDay: number;
  /** Last instant the queue may post (end of the end date's day). */
  windowEnd: string;
  /** Time the next automatic post is due, or null before the first post. */
  nextPostAt: string | null;
}

export type BackupFrequency = 'off' | 'hourly' | '6h' | '12h' | 'daily' | 'weekly';

/** Full-system backup settings and status (admin). */
export interface BackupConfig {
  frequency: BackupFrequency;
  /** Number of backups kept in the bucket. */
  retention: number;
  passphraseSet: boolean;
  /** False when no S3 bucket is configured on the server. */
  storageConfigured: boolean;
  /** A backup or restore is in progress. */
  running: boolean;
  minPassphraseLength: number;
  lastAttemptAt?: string;
  lastSuccessAt?: string;
  lastError?: string;
  lastName?: string;
}

/** One backup archive stored in the bucket. */
export interface BackupEntry {
  name: string;
  size: number;
  createdAt: string;
  /** scheduled, manual or pre-restore */
  kind: string;
}

export interface BackupRestoreResult {
  restored: boolean;
  /** Name of the backup of the state before the restore, if one was taken. */
  safetyBackup: string;
  collections: Record<string, number>;
}

/** One upload no document references any more. */
export interface OrphanedUpload {
  filename: string;
  size: number;
  createdAt: string;
  /** Where the orphan still exists: normally a database record with its
   *  bytes in the S3 bucket (or in the record without one), plus a disk cache. */
  onDisk: boolean;
  inDatabase: boolean;
  inBucket: boolean;
}

/** Result of an admin storage scan or sweep. */
export interface UploadSweepReport {
  /** True when nothing was deleted. */
  dryRun: boolean;
  /** Age guard the run used: uploads newer than this were left alone. */
  minAgeHours: number;
  totalFiles: number;
  totalBytes: number;
  orphanedFiles: number;
  orphanedBytes: number;
  /** Unreferenced uploads the age guard spared. */
  skippedTooRecent: number;
  /** Zero on a dry run. */
  deletedFiles: number;
  deletedBytes: number;
  /** Up to 50 of the orphans, newest first. */
  sample: OrphanedUpload[];
  errors?: string[];
}

// An authorization request from an MCP client, as shown on the consent page.
// When the request is invalid in a way the client should hear about, only
// redirectUrl is set and the page sends the browser straight back.
export interface OAuthAuthorization {
  client?: { name: string; uri: string; logoUri: string };
  redirectHost?: string;
  scopes?: string[];
  user?: { name: string; email: string };
  redirectUrl?: string;
}

// An app the user has connected to the MCP server through OAuth.
export interface OAuthGrant {
  clientId: string;
  clientName: string;
  lastUsedAt: string;
}

// ─── Instagram growth ────────────────────────────────────────────────────────

export type SlotFormat = 'reel' | 'carousel' | 'image' | 'story' | 'any';

export interface WeeklyPlanSlot {
  id: string;
  /** 0 = Monday … 6 = Sunday */
  weekday: number;
  time?: string;
  format: SlotFormat;
  label?: string;
}

export interface InstagramGoal {
  target: number;
  startDate: string;
  endDate: string;
  startFollowers?: number;
  halfwayDate?: string;
  halfwayThreshold?: number;
}

export interface GrowthSettings {
  instagramGoal?: InstagramGoal;
  weeklyPlan: WeeklyPlanSlot[];
  maxSingleImagesPerWeek?: number;
  instagramRewritePrompt?: string;
  /** Open drafts MCP clients and agents may have waiting for review; 0 = no limit. */
  maxAgentDrafts?: number;
}

export interface GoalWeek {
  start: string;
  end: string;
  net: number | null;
  required: number;
  current: boolean;
}

export interface GoalProgress {
  goal: InstagramGoal;
  startFollowers: number | null;
  currentFollowers: number | null;
  currentDate?: string;
  requiredPerWeek: number;
  totalWeeks: number;
  progress: number | null;
  expectedToday: number | null;
  series: { date: string; followers: number }[];
  weeks: GoalWeek[];
  halfway: 'pending' | 'ok' | 'below';
  halfwayDate: string;
  halfwayProgress?: number;
}

export type ReportFormat = 'reel' | 'carousel' | 'image';

export interface PostPerformance {
  postId: string;
  format: ReportFormat | 'story';
  postedAt: string;
  caption: string;
  imageUrl?: string;
  permalink?: string;
  reach: number;
  views: number;
  shares: number;
  saved: number;
  likes: number;
  comments: number;
  follows: number;
  profileVisits: number;
  sendsPerReach: number;
  savesPerReach: number;
  score: number;
  hasInsights: boolean;
  insightsError?: string;
  adCandidate: boolean;
}

export interface FormatStats {
  format: ReportFormat;
  posts: number;
  withInsights: number;
  reach: number;
  shares: number;
  saved: number;
  follows: number;
  avgReach: number;
  sendsPerReach: number;
  savesPerReach: number;
}

export interface WeeklyReport {
  start: string;
  end: string;
  posts: PostPerformance[];
  topPosts: PostPerformance[];
  formats: FormatStats[];
  weakestFormat?: ReportFormat;
  stories: number;
  missingInsights: number;
  followersStart?: number;
  followersEnd?: number;
  netFollowers?: number;
}

export interface GrowthOverview {
  accounts: { id: string; accountName: string; avatarUrl?: string }[];
  accountId?: string;
  settings: GrowthSettings;
  goal?: GoalProgress;
  latestSnapshot?: { date: string; followersCount: number; followsCount: number; mediaCount: number; updatedAt: string };
  report: WeeklyReport;
  adCandidates: PostPerformance[];
}

// ─── Agents ──────────────────────────────────────────────────────────────────

export type AgentScheduleKind = 'manual' | 'interval' | 'hourly' | 'daily';

export interface AgentSchedule {
  kind: AgentScheduleKind;
  intervalMinutes?: number;
  minute?: number;
  /** "HH:MM", for daily schedules */
  times?: string[];
  /** 0 = Monday; empty = every day */
  weekdays?: number[];
  timezone?: string;
}

export interface Agent {
  id: string;
  name: string;
  prompt: string;
  enabled: boolean;
  schedule: AgentSchedule;
  /** Overrides the default model from the Profile page */
  model?: string;
  maxSteps?: number;
  /** Origin label of the drafts it creates */
  origin: string;
  nextRunAt?: string;
  running: boolean;
  runStartedAt?: string;
  lastRunAt?: string;
  lastStatus?: AgentRunStatus;
  lastError?: string;
}

export type AgentInput = Pick<Agent, 'name' | 'prompt' | 'enabled' | 'schedule' | 'model' | 'maxSteps'>;

export type AgentRunStatus = 'running' | 'ok' | 'error' | 'limit' | 'stopped';

export interface AgentRunStep {
  at: string;
  kind: 'message' | 'tool';
  text?: string;
  tool?: string;
  args?: string;
  result?: string;
  isError?: boolean;
}

export interface AgentRun {
  id: string;
  agentId: string;
  trigger: 'schedule' | 'manual';
  model: string;
  status: AgentRunStatus;
  startedAt: string;
  finishedAt?: string;
  steps?: AgentRunStep[];
  toolCalls: number;
  summary?: string;
  error?: string;
  promptTokens: number;
  completionTokens: number;
  cost: number;
}

export interface AgentSettings {
  hasKey: boolean;
  keyHint: string;
  model: string;
  globalPrompt: string;
}

export interface AgentModel {
  id: string;
  name: string;
  contextLength: number;
  /** US dollars per million tokens */
  promptPrice: number;
  completionPrice: number;
  images: boolean;
}
