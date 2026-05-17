"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.SocialPod = void 0;
const n8n_workflow_1 = require("n8n-workflow");
const form_data_1 = __importDefault(require("form-data"));
// Sends a multipart/form-data request with post data in the `data` JSON field,
// optionally attaching binary image buffers as `images` fields.
async function multipartRequest(ctx, method, url, data, images) {
    const form = new form_data_1.default();
    form.append('data', JSON.stringify(data));
    if (images) {
        for (const img of images) {
            form.append('images', img.buffer, { filename: img.fileName, contentType: img.mimeType });
        }
    }
    return ctx.helpers.httpRequestWithAuthentication.call(ctx, 'socialPodApi', {
        method,
        url,
        body: form,
        headers: form.getHeaders(),
    });
}
// Parses a comma-separated string into a trimmed, non-empty string array.
function parseList(value) {
    return value.split(',').map((s) => s.trim()).filter(Boolean);
}
class SocialPod {
    constructor() {
        this.description = {
            displayName: 'SocialPod',
            name: 'socialPod',
            icon: 'file:socialpod.svg',
            group: ['output'],
            version: 1,
            subtitle: '={{$parameter["operation"] + ": " + $parameter["resource"]}}',
            description: 'Schedule and manage social media posts via SocialPod',
            defaults: { name: 'SocialPod' },
            inputs: ['main'],
            outputs: ['main'],
            credentials: [{ name: 'socialPodApi', required: true }],
            properties: [
                // ── Resource ──────────────────────────────────────────────────────
                {
                    displayName: 'Resource',
                    name: 'resource',
                    type: 'options',
                    noDataExpression: true,
                    options: [
                        { name: 'Post', value: 'post' },
                        { name: 'Suffix', value: 'suffix' },
                    ],
                    default: 'post',
                },
                // ── Post: operation ───────────────────────────────────────────────
                {
                    displayName: 'Operation',
                    name: 'operation',
                    type: 'options',
                    noDataExpression: true,
                    displayOptions: { show: { resource: ['post'] } },
                    options: [
                        { name: 'Create', value: 'create', action: 'Create a post' },
                        { name: 'Delete', value: 'delete', action: 'Delete a post' },
                        { name: 'Get', value: 'get', action: 'Get a post by ID' },
                        { name: 'List', value: 'list', action: 'List posts' },
                        { name: 'Reschedule', value: 'reschedule', action: 'Reschedule a post' },
                        { name: 'Update', value: 'update', action: 'Update a post' },
                    ],
                    default: 'create',
                },
                // ── Suffix: operation ─────────────────────────────────────────────
                {
                    displayName: 'Operation',
                    name: 'operation',
                    type: 'options',
                    noDataExpression: true,
                    displayOptions: { show: { resource: ['suffix'] } },
                    options: [
                        { name: 'Create', value: 'create', action: 'Create a suffix' },
                        { name: 'Delete', value: 'delete', action: 'Delete a suffix' },
                        { name: 'List', value: 'list', action: 'List suffixes' },
                        { name: 'Update', value: 'update', action: 'Update a suffix' },
                    ],
                    default: 'list',
                },
                // ── Shared: Post ID ───────────────────────────────────────────────
                {
                    displayName: 'Post ID',
                    name: 'postId',
                    type: 'string',
                    required: true,
                    displayOptions: {
                        show: { resource: ['post'], operation: ['get', 'update', 'delete', 'reschedule'] },
                    },
                    default: '',
                    description: 'ID of the post to act on',
                },
                // ── Shared: Suffix ID ─────────────────────────────────────────────
                {
                    displayName: 'Suffix ID',
                    name: 'suffixId',
                    type: 'string',
                    required: true,
                    displayOptions: {
                        show: { resource: ['suffix'], operation: ['update', 'delete'] },
                    },
                    default: '',
                    description: 'ID of the suffix to act on',
                },
                // ── Post: Create — required fields ────────────────────────────────
                {
                    displayName: 'Content',
                    name: 'content',
                    type: 'string',
                    typeOptions: { rows: 4 },
                    required: true,
                    displayOptions: { show: { resource: ['post'], operation: ['create'] } },
                    default: '',
                    description: 'Text content of the post',
                },
                {
                    displayName: 'Platforms',
                    name: 'platforms',
                    type: 'multiOptions',
                    required: true,
                    displayOptions: { show: { resource: ['post'], operation: ['create'] } },
                    options: [
                        { name: 'Bluesky', value: 'bluesky' },
                        { name: 'Instagram', value: 'instagram' },
                    ],
                    default: ['bluesky'],
                    description: 'Platforms to publish this post on',
                },
                {
                    displayName: 'Scheduled At',
                    name: 'scheduledAt',
                    type: 'string',
                    required: true,
                    displayOptions: { show: { resource: ['post'], operation: ['create'] } },
                    default: '',
                    placeholder: '2025-06-01T09:00:00Z',
                    description: 'ISO 8601 datetime when the post should be published',
                },
                {
                    displayName: 'Post Type',
                    name: 'postType',
                    type: 'options',
                    displayOptions: { show: { resource: ['post'], operation: ['create'] } },
                    options: [
                        { name: 'Post', value: 'post' },
                        { name: 'Reel', value: 'reel' },
                        { name: 'Story', value: 'story' },
                    ],
                    default: 'post',
                    description: 'Type of content to publish. Reels and Stories are Instagram-only and require a video or image file.',
                },
                {
                    displayName: 'Status',
                    name: 'status',
                    type: 'options',
                    displayOptions: { show: { resource: ['post'], operation: ['create'] } },
                    options: [
                        { name: 'Scheduled', value: 'scheduled' },
                        { name: 'Draft', value: 'draft' },
                    ],
                    default: 'scheduled',
                    description: 'Scheduled posts are published automatically; drafts are not',
                },
                // ── Post: Create — additional fields ──────────────────────────────
                {
                    displayName: 'Additional Fields',
                    name: 'additionalFields',
                    type: 'collection',
                    placeholder: 'Add Field',
                    displayOptions: { show: { resource: ['post'], operation: ['create'] } },
                    default: {},
                    options: [
                        {
                            displayName: 'Binary Image Property',
                            name: 'binaryProperty',
                            type: 'string',
                            default: 'data',
                            description: 'Name of the binary property containing the image to attach (from a previous node)',
                        },
                        {
                            displayName: 'Image URLs',
                            name: 'imageUrls',
                            type: 'string',
                            default: '',
                            placeholder: '/api/uploads/abc.jpg, /api/uploads/def.png',
                            description: 'Comma-separated list of image URLs already uploaded to SocialPod',
                        },
                        {
                            displayName: 'Bluesky Suffix ID',
                            name: 'bluskySuffixId',
                            type: 'string',
                            default: '',
                            description: 'ID of the suffix to append when publishing to Bluesky',
                        },
                        {
                            displayName: 'Instagram Suffix ID',
                            name: 'instagramSuffixId',
                            type: 'string',
                            default: '',
                            description: 'ID of the suffix to append when publishing to Instagram',
                        },
                        {
                            displayName: 'First Comment',
                            name: 'firstComment',
                            type: 'string',
                            typeOptions: { rows: 2 },
                            default: '',
                            description: 'Text to post as the first comment immediately after publishing',
                        },
                        {
                            displayName: 'Bluesky Account ID',
                            name: 'blueskyAccountId',
                            type: 'string',
                            default: '',
                            description: 'ID of the Bluesky account to post from (leave empty to use the default account)',
                        },
                        {
                            displayName: 'Instagram Account ID',
                            name: 'instagramAccountId',
                            type: 'string',
                            default: '',
                            description: 'ID of the Instagram account to post from (leave empty to use the default account)',
                        },
                        {
                            displayName: 'Bluesky Content Override',
                            name: 'blueskyContentOverride',
                            type: 'string',
                            typeOptions: { rows: 4 },
                            default: '',
                            description: 'Custom caption for Bluesky only — overrides the shared Content field',
                        },
                        {
                            displayName: 'Instagram Content Override',
                            name: 'instagramContentOverride',
                            type: 'string',
                            typeOptions: { rows: 4 },
                            default: '',
                            description: 'Custom caption for Instagram only — overrides the shared Content field',
                        },
                        {
                            displayName: 'Tags',
                            name: 'tags',
                            type: 'string',
                            default: '',
                            placeholder: 'marketing, campaign-q1',
                            description: 'Comma-separated list of tags for internal organisation',
                        },
                    ],
                },
                // ── Post: Update ──────────────────────────────────────────────────
                {
                    displayName: 'Update Fields',
                    name: 'updateFields',
                    type: 'collection',
                    placeholder: 'Add Field',
                    displayOptions: { show: { resource: ['post'], operation: ['update'] } },
                    default: {},
                    options: [
                        {
                            displayName: 'Content',
                            name: 'content',
                            type: 'string',
                            typeOptions: { rows: 4 },
                            default: '',
                        },
                        {
                            displayName: 'Post Type',
                            name: 'postType',
                            type: 'options',
                            options: [
                                { name: 'Post', value: 'post' },
                                { name: 'Reel', value: 'reel' },
                                { name: 'Story', value: 'story' },
                            ],
                            default: 'post',
                            description: 'Type of content to publish. Reels and Stories are Instagram-only.',
                        },
                        {
                            displayName: 'Platforms',
                            name: 'platforms',
                            type: 'multiOptions',
                            options: [
                                { name: 'Bluesky', value: 'bluesky' },
                                { name: 'Instagram', value: 'instagram' },
                            ],
                            default: [],
                        },
                        {
                            displayName: 'Scheduled At',
                            name: 'scheduledAt',
                            type: 'string',
                            default: '',
                            placeholder: '2025-06-01T09:00:00Z',
                        },
                        {
                            displayName: 'Status',
                            name: 'status',
                            type: 'options',
                            options: [
                                { name: 'Scheduled', value: 'scheduled' },
                                { name: 'Draft', value: 'draft' },
                            ],
                            default: 'scheduled',
                        },
                        {
                            displayName: 'Binary Image Property',
                            name: 'binaryProperty',
                            type: 'string',
                            default: 'data',
                            description: 'Name of the binary property containing the image to attach',
                        },
                        {
                            displayName: 'Image URLs',
                            name: 'imageUrls',
                            type: 'string',
                            default: '',
                            description: 'Comma-separated list of image URLs (replaces existing images)',
                        },
                        {
                            displayName: 'Bluesky Suffix ID',
                            name: 'bluskySuffixId',
                            type: 'string',
                            default: '',
                            description: 'Set to the suffix ID to apply, or leave empty to remove',
                        },
                        {
                            displayName: 'Instagram Suffix ID',
                            name: 'instagramSuffixId',
                            type: 'string',
                            default: '',
                            description: 'Set to the suffix ID to apply, or leave empty to remove',
                        },
                        {
                            displayName: 'First Comment',
                            name: 'firstComment',
                            type: 'string',
                            typeOptions: { rows: 2 },
                            default: '',
                            description: 'Text to post as the first comment immediately after publishing',
                        },
                        {
                            displayName: 'Bluesky Account ID',
                            name: 'blueskyAccountId',
                            type: 'string',
                            default: '',
                            description: 'ID of the Bluesky account to post from (leave empty to use the default account)',
                        },
                        {
                            displayName: 'Instagram Account ID',
                            name: 'instagramAccountId',
                            type: 'string',
                            default: '',
                            description: 'ID of the Instagram account to post from (leave empty to use the default account)',
                        },
                        {
                            displayName: 'Bluesky Content Override',
                            name: 'blueskyContentOverride',
                            type: 'string',
                            typeOptions: { rows: 4 },
                            default: '',
                            description: 'Custom caption for Bluesky only — overrides the shared Content field',
                        },
                        {
                            displayName: 'Instagram Content Override',
                            name: 'instagramContentOverride',
                            type: 'string',
                            typeOptions: { rows: 4 },
                            default: '',
                            description: 'Custom caption for Instagram only — overrides the shared Content field',
                        },
                        {
                            displayName: 'Tags',
                            name: 'tags',
                            type: 'string',
                            default: '',
                            placeholder: 'marketing, campaign-q1',
                            description: 'Comma-separated list of tags for internal organisation',
                        },
                    ],
                },
                // ── Post: Reschedule ──────────────────────────────────────────────
                {
                    displayName: 'New Scheduled At',
                    name: 'scheduledAt',
                    type: 'string',
                    required: true,
                    displayOptions: { show: { resource: ['post'], operation: ['reschedule'] } },
                    default: '',
                    placeholder: '2025-06-01T09:00:00Z',
                    description: 'New ISO 8601 datetime for the post',
                },
                // ── Post: List — filters ──────────────────────────────────────────
                {
                    displayName: 'Filters',
                    name: 'filters',
                    type: 'collection',
                    placeholder: 'Add Filter',
                    displayOptions: { show: { resource: ['post'], operation: ['list'] } },
                    default: {},
                    options: [
                        {
                            displayName: 'Start',
                            name: 'start',
                            type: 'string',
                            default: '',
                            placeholder: '2025-01-01T00:00:00Z',
                            description: 'Return posts scheduled on or after this datetime (ISO 8601)',
                        },
                        {
                            displayName: 'End',
                            name: 'end',
                            type: 'string',
                            default: '',
                            placeholder: '2025-12-31T23:59:59Z',
                            description: 'Return posts scheduled on or before this datetime (ISO 8601)',
                        },
                        {
                            displayName: 'Status',
                            name: 'status',
                            type: 'options',
                            options: [
                                { name: 'Any', value: '' },
                                { name: 'Draft', value: 'draft' },
                                { name: 'Failed', value: 'failed' },
                                { name: 'Published', value: 'published' },
                                { name: 'Scheduled', value: 'scheduled' },
                            ],
                            default: '',
                        },
                        {
                            displayName: 'Platform',
                            name: 'platform',
                            type: 'options',
                            options: [
                                { name: 'Any', value: '' },
                                { name: 'Bluesky', value: 'bluesky' },
                                { name: 'Instagram', value: 'instagram' },
                            ],
                            default: '',
                        },
                    ],
                },
                // ── Suffix: Create ────────────────────────────────────────────────
                {
                    displayName: 'Name',
                    name: 'name',
                    type: 'string',
                    required: true,
                    displayOptions: { show: { resource: ['suffix'], operation: ['create'] } },
                    default: '',
                    description: 'Short display name for this suffix (e.g. "Website footer")',
                },
                {
                    displayName: 'Content',
                    name: 'content',
                    type: 'string',
                    typeOptions: { rows: 3 },
                    required: true,
                    displayOptions: { show: { resource: ['suffix'], operation: ['create'] } },
                    default: '',
                    description: 'Text that will be appended to posts at publish time',
                },
                // ── Suffix: Update ────────────────────────────────────────────────
                {
                    displayName: 'Update Fields',
                    name: 'updateFields',
                    type: 'collection',
                    placeholder: 'Add Field',
                    displayOptions: { show: { resource: ['suffix'], operation: ['update'] } },
                    default: {},
                    options: [
                        {
                            displayName: 'Name',
                            name: 'name',
                            type: 'string',
                            default: '',
                        },
                        {
                            displayName: 'Content',
                            name: 'content',
                            type: 'string',
                            typeOptions: { rows: 3 },
                            default: '',
                        },
                    ],
                },
            ],
        };
    }
    async execute() {
        var _a, _b, _c;
        const items = this.getInputData();
        const returnData = [];
        const credentials = await this.getCredentials('socialPodApi');
        const baseUrl = credentials.apiUrl.replace(/\/$/, '');
        for (let i = 0; i < items.length; i++) {
            try {
                const resource = this.getNodeParameter('resource', i);
                const operation = this.getNodeParameter('operation', i);
                let responseData;
                // ── Post ────────────────────────────────────────────────────────
                if (resource === 'post') {
                    if (operation === 'create') {
                        const content = this.getNodeParameter('content', i);
                        const platforms = this.getNodeParameter('platforms', i);
                        const scheduledAt = this.getNodeParameter('scheduledAt', i);
                        const postType = this.getNodeParameter('postType', i);
                        const status = this.getNodeParameter('status', i);
                        const extra = this.getNodeParameter('additionalFields', i, {});
                        const body = { content, platforms, scheduledAt, postType, status };
                        if (extra.imageUrls)
                            body.imageUrls = parseList(extra.imageUrls);
                        const suffixIds = {};
                        if (extra.bluskySuffixId)
                            suffixIds.bluesky = extra.bluskySuffixId;
                        if (extra.instagramSuffixId)
                            suffixIds.instagram = extra.instagramSuffixId;
                        if (Object.keys(suffixIds).length)
                            body.suffixIds = suffixIds;
                        if (extra.firstComment)
                            body.firstComment = extra.firstComment;
                        if (extra.tags)
                            body.tags = parseList(extra.tags);
                        const accountIds = {};
                        if (extra.blueskyAccountId)
                            accountIds.bluesky = extra.blueskyAccountId;
                        if (extra.instagramAccountId)
                            accountIds.instagram = extra.instagramAccountId;
                        if (Object.keys(accountIds).length)
                            body.accountIds = accountIds;
                        const contentOverrides = {};
                        if (extra.blueskyContentOverride)
                            contentOverrides.bluesky = extra.blueskyContentOverride;
                        if (extra.instagramContentOverride)
                            contentOverrides.instagram = extra.instagramContentOverride;
                        if (Object.keys(contentOverrides).length)
                            body.contentOverrides = contentOverrides;
                        // Attach binary image from a previous workflow step
                        const images = [];
                        if (extra.binaryProperty) {
                            const binaryProp = extra.binaryProperty;
                            const binaryData = (_a = items[i].binary) === null || _a === void 0 ? void 0 : _a[binaryProp];
                            if (binaryData) {
                                const buffer = await this.helpers.getBinaryDataBuffer(i, binaryProp);
                                images.push({
                                    buffer,
                                    fileName: binaryData.fileName || 'image.png',
                                    mimeType: binaryData.mimeType || 'image/png',
                                });
                            }
                        }
                        responseData = await multipartRequest(this, 'POST', `${baseUrl}/api/posts`, body, images.length ? images : undefined);
                    }
                    else if (operation === 'get') {
                        const postId = this.getNodeParameter('postId', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'GET',
                            url: `${baseUrl}/api/posts/${postId}`,
                        });
                    }
                    else if (operation === 'list') {
                        const filters = this.getNodeParameter('filters', i, {});
                        const qs = {};
                        if (filters.start)
                            qs.start = filters.start;
                        if (filters.end)
                            qs.end = filters.end;
                        if (filters.status)
                            qs.status = filters.status;
                        if (filters.platform)
                            qs.platform = filters.platform;
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'GET',
                            url: `${baseUrl}/api/posts`,
                            qs,
                        });
                    }
                    else if (operation === 'update') {
                        const postId = this.getNodeParameter('postId', i);
                        const fields = this.getNodeParameter('updateFields', i, {});
                        const body = {};
                        if (fields.content !== undefined && fields.content !== '')
                            body.content = fields.content;
                        if (fields.postType !== undefined && fields.postType !== '')
                            body.postType = fields.postType;
                        if (fields.scheduledAt !== undefined && fields.scheduledAt !== '')
                            body.scheduledAt = fields.scheduledAt;
                        if (fields.status !== undefined && fields.status !== '')
                            body.status = fields.status;
                        if (fields.imageUrls !== undefined && fields.imageUrls !== '')
                            body.imageUrls = parseList(fields.imageUrls);
                        if ((_b = fields.platforms) === null || _b === void 0 ? void 0 : _b.length)
                            body.platforms = fields.platforms;
                        const suffixIds = {};
                        if (fields.bluskySuffixId)
                            suffixIds.bluesky = fields.bluskySuffixId;
                        if (fields.instagramSuffixId)
                            suffixIds.instagram = fields.instagramSuffixId;
                        body.suffixIds = suffixIds; // always send — empty map clears suffixes
                        if (fields.firstComment !== undefined && fields.firstComment !== '')
                            body.firstComment = fields.firstComment;
                        if (fields.tags)
                            body.tags = parseList(fields.tags);
                        const accountIds = {};
                        if (fields.blueskyAccountId)
                            accountIds.bluesky = fields.blueskyAccountId;
                        if (fields.instagramAccountId)
                            accountIds.instagram = fields.instagramAccountId;
                        if (Object.keys(accountIds).length)
                            body.accountIds = accountIds;
                        const contentOverrides = {};
                        if (fields.blueskyContentOverride)
                            contentOverrides.bluesky = fields.blueskyContentOverride;
                        if (fields.instagramContentOverride)
                            contentOverrides.instagram = fields.instagramContentOverride;
                        body.contentOverrides = contentOverrides; // always send — empty map clears overrides
                        // Attach binary image from a previous workflow step
                        const images = [];
                        if (fields.binaryProperty) {
                            const binaryProp = fields.binaryProperty;
                            const binaryData = (_c = items[i].binary) === null || _c === void 0 ? void 0 : _c[binaryProp];
                            if (binaryData) {
                                const buffer = await this.helpers.getBinaryDataBuffer(i, binaryProp);
                                images.push({
                                    buffer,
                                    fileName: binaryData.fileName || 'image.png',
                                    mimeType: binaryData.mimeType || 'image/png',
                                });
                            }
                        }
                        responseData = await multipartRequest(this, 'PUT', `${baseUrl}/api/posts/${postId}`, body, images.length ? images : undefined);
                    }
                    else if (operation === 'delete') {
                        const postId = this.getNodeParameter('postId', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'DELETE',
                            url: `${baseUrl}/api/posts/${postId}`,
                        });
                    }
                    else if (operation === 'reschedule') {
                        const postId = this.getNodeParameter('postId', i);
                        const scheduledAt = this.getNodeParameter('scheduledAt', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'PATCH',
                            url: `${baseUrl}/api/posts/${postId}/reschedule`,
                            body: { scheduledAt },
                            headers: { 'Content-Type': 'application/json' },
                        });
                    }
                    else {
                        throw new n8n_workflow_1.NodeOperationError(this.getNode(), `Unknown post operation: ${operation}`);
                    }
                    // ── Suffix ──────────────────────────────────────────────────────
                }
                else if (resource === 'suffix') {
                    if (operation === 'list') {
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'GET',
                            url: `${baseUrl}/api/suffixes`,
                        });
                    }
                    else if (operation === 'create') {
                        const name = this.getNodeParameter('name', i);
                        const content = this.getNodeParameter('content', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'POST',
                            url: `${baseUrl}/api/suffixes`,
                            body: { name, content },
                            headers: { 'Content-Type': 'application/json' },
                        });
                    }
                    else if (operation === 'update') {
                        const suffixId = this.getNodeParameter('suffixId', i);
                        const fields = this.getNodeParameter('updateFields', i, {});
                        const body = {};
                        if (fields.name !== undefined && fields.name !== '')
                            body.name = fields.name;
                        if (fields.content !== undefined && fields.content !== '')
                            body.content = fields.content;
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'PUT',
                            url: `${baseUrl}/api/suffixes/${suffixId}`,
                            body,
                            headers: { 'Content-Type': 'application/json' },
                        });
                    }
                    else if (operation === 'delete') {
                        const suffixId = this.getNodeParameter('suffixId', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'DELETE',
                            url: `${baseUrl}/api/suffixes/${suffixId}`,
                        });
                    }
                    else {
                        throw new n8n_workflow_1.NodeOperationError(this.getNode(), `Unknown suffix operation: ${operation}`);
                    }
                }
                else {
                    throw new n8n_workflow_1.NodeOperationError(this.getNode(), `Unknown resource: ${resource}`);
                }
                const rows = Array.isArray(responseData) ? responseData : [responseData];
                returnData.push(...rows.map((json) => ({ json, pairedItem: { item: i } })));
            }
            catch (error) {
                if (this.continueOnFail()) {
                    returnData.push({
                        json: { error: error.message },
                        pairedItem: { item: i },
                    });
                    continue;
                }
                throw error;
            }
        }
        return [returnData];
    }
}
exports.SocialPod = SocialPod;
