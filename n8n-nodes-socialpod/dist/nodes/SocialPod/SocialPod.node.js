"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.SocialPod = void 0;
const n8n_workflow_1 = require("n8n-workflow");
const form_data_1 = __importDefault(require("form-data"));
const ALL_PLATFORMS = [
    { name: 'Bluesky', value: 'bluesky' },
    { name: 'Instagram', value: 'instagram' },
    { name: 'LinkedIn', value: 'linkedin' },
    { name: 'Mastodon', value: 'mastodon' },
    { name: 'Threads', value: 'threads' },
    { name: 'Twitter/X', value: 'twitter' },
    { name: 'YouTube', value: 'youtube' },
];
const ALL_PLATFORMS_FILTER = [
    { name: 'Any', value: '' },
    ...ALL_PLATFORMS,
];
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
function parseList(value) {
    return value.split(',').map((s) => s.trim()).filter(Boolean);
}
function collectPlatformMap(fields, fieldSuffix, platforms) {
    const map = {};
    for (const p of platforms) {
        const key = `${p}${fieldSuffix}`;
        if (fields[key])
            map[p] = fields[key];
    }
    return map;
}
const PLATFORM_KEYS = ['bluesky', 'instagram', 'linkedin', 'mastodon', 'threads', 'twitter', 'youtube'];
function platformSuffixFields(displayPrefix, fieldSuffix, descSuffix) {
    return PLATFORM_KEYS.map((p) => ({
        displayName: `${displayPrefix} ${p.charAt(0).toUpperCase() + p.slice(1)}`,
        name: `${p}${fieldSuffix}`,
        type: 'string',
        default: '',
        description: `${descSuffix} for ${p.charAt(0).toUpperCase() + p.slice(1)}`,
    }));
}
function platformContentOverrideFields() {
    return PLATFORM_KEYS.map((p) => ({
        displayName: `${p.charAt(0).toUpperCase() + p.slice(1)} Content Override`,
        name: `${p}ContentOverride`,
        type: 'string',
        typeOptions: { rows: 4 },
        default: '',
        description: `Custom caption for ${p.charAt(0).toUpperCase() + p.slice(1)} only — overrides the shared Content field`,
    }));
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
                        { name: 'Account', value: 'account' },
                        { name: 'AI Text', value: 'aiText' },
                        { name: 'Mention', value: 'mention' },
                        { name: 'Post', value: 'post' },
                        { name: 'Suffix', value: 'suffix' },
                        { name: 'Watermark', value: 'watermark' },
                    ],
                    default: 'post',
                },
                // ── Account: operation ────────────────────────────────────────────
                {
                    displayName: 'Operation',
                    name: 'operation',
                    type: 'options',
                    noDataExpression: true,
                    displayOptions: { show: { resource: ['account'] } },
                    options: [
                        { name: 'List', value: 'list', action: 'List connected social accounts' },
                    ],
                    default: 'list',
                },
                // ── AI Text: operation ────────────────────────────────────────────
                {
                    displayName: 'Operation',
                    name: 'operation',
                    type: 'options',
                    noDataExpression: true,
                    displayOptions: { show: { resource: ['aiText'] } },
                    options: [
                        { name: 'Generate', value: 'generate', action: 'Generate text with AI' },
                    ],
                    default: 'generate',
                },
                // ── AI Text: Generate — fields ────────────────────────────────────
                {
                    displayName: 'Prompt',
                    name: 'prompt',
                    type: 'string',
                    typeOptions: { rows: 4 },
                    required: true,
                    displayOptions: { show: { resource: ['aiText'], operation: ['generate'] } },
                    default: '',
                    description: 'Instructions for the AI text generator',
                },
                {
                    displayName: 'Platforms',
                    name: 'platforms',
                    type: 'multiOptions',
                    displayOptions: { show: { resource: ['aiText'], operation: ['generate'] } },
                    options: [...ALL_PLATFORMS],
                    default: [],
                    description: 'Optional platforms to tailor the generated text for',
                },
                // ── Mention: operation ────────────────────────────────────────────
                {
                    displayName: 'Operation',
                    name: 'operation',
                    type: 'options',
                    noDataExpression: true,
                    displayOptions: { show: { resource: ['mention'] } },
                    options: [
                        { name: 'Create', value: 'create', action: 'Create a mention' },
                        { name: 'Delete', value: 'delete', action: 'Delete a mention' },
                        { name: 'Export', value: 'export', action: 'Export all mentions as JSON' },
                        { name: 'Import', value: 'import', action: 'Import mentions from JSON' },
                        { name: 'List', value: 'list', action: 'List mentions' },
                        { name: 'Update', value: 'update', action: 'Update a mention' },
                    ],
                    default: 'list',
                },
                // ── Mention: ID ──────────────────────────────────────────────────
                {
                    displayName: 'Mention ID',
                    name: 'mentionId',
                    type: 'string',
                    required: true,
                    displayOptions: {
                        show: { resource: ['mention'], operation: ['update', 'delete'] },
                    },
                    default: '',
                    description: 'ID of the mention to act on',
                },
                // ── Mention: Create ──────────────────────────────────────────────
                {
                    displayName: 'Name',
                    name: 'name',
                    type: 'string',
                    required: true,
                    displayOptions: { show: { resource: ['mention'], operation: ['create'] } },
                    default: '',
                    description: 'Display name for this mention entry (e.g. publisher or person name)',
                },
                {
                    displayName: 'Additional Fields',
                    name: 'additionalFields',
                    type: 'collection',
                    placeholder: 'Add Field',
                    displayOptions: { show: { resource: ['mention'], operation: ['create'] } },
                    default: {},
                    options: [
                        {
                            displayName: 'Country',
                            name: 'country',
                            type: 'string',
                            default: '',
                            description: 'Country of the mention entry',
                        },
                        ...platformSuffixFields('Handle', 'Handle', 'Platform handle/username'),
                    ],
                },
                // ── Mention: Update ──────────────────────────────────────────────
                {
                    displayName: 'Update Fields',
                    name: 'updateFields',
                    type: 'collection',
                    placeholder: 'Add Field',
                    displayOptions: { show: { resource: ['mention'], operation: ['update'] } },
                    default: {},
                    options: [
                        {
                            displayName: 'Name',
                            name: 'name',
                            type: 'string',
                            default: '',
                        },
                        {
                            displayName: 'Country',
                            name: 'country',
                            type: 'string',
                            default: '',
                        },
                        ...platformSuffixFields('Handle', 'Handle', 'Platform handle/username'),
                    ],
                },
                // ── Mention: Import ──────────────────────────────────────────────
                {
                    displayName: 'JSON Data',
                    name: 'jsonData',
                    type: 'string',
                    typeOptions: { rows: 6 },
                    required: true,
                    displayOptions: { show: { resource: ['mention'], operation: ['import'] } },
                    default: '',
                    description: 'JSON array of mention entries to import (same format as the export)',
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
                        { name: 'Retry', value: 'retry', action: 'Retry a failed post' },
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
                // ── Watermark: operation ──────────────────────────────────────────
                {
                    displayName: 'Operation',
                    name: 'operation',
                    type: 'options',
                    noDataExpression: true,
                    displayOptions: { show: { resource: ['watermark'] } },
                    options: [
                        { name: 'Delete', value: 'delete', action: 'Delete a watermark' },
                        { name: 'List', value: 'list', action: 'List watermarks' },
                    ],
                    default: 'list',
                },
                // ── Watermark: ID ────────────────────────────────────────────────
                {
                    displayName: 'Watermark ID',
                    name: 'watermarkId',
                    type: 'string',
                    required: true,
                    displayOptions: {
                        show: { resource: ['watermark'], operation: ['delete'] },
                    },
                    default: '',
                    description: 'ID of the watermark to delete',
                },
                // ── Shared: Post ID ───────────────────────────────────────────────
                {
                    displayName: 'Post ID',
                    name: 'postId',
                    type: 'string',
                    required: true,
                    displayOptions: {
                        show: { resource: ['post'], operation: ['get', 'update', 'delete', 'reschedule', 'retry'] },
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
                    options: [...ALL_PLATFORMS],
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
                        ...platformSuffixFields('Suffix ID', 'SuffixId', 'ID of the suffix to append when publishing'),
                        {
                            displayName: 'First Comment',
                            name: 'firstComment',
                            type: 'string',
                            typeOptions: { rows: 2 },
                            default: '',
                            description: 'Text to post as the first comment immediately after publishing',
                        },
                        ...platformSuffixFields('Account ID', 'AccountId', 'ID of the account to post from (leave empty for default)'),
                        ...platformContentOverrideFields(),
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
                            options: [...ALL_PLATFORMS],
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
                        ...platformSuffixFields('Suffix ID', 'SuffixId', 'Set to the suffix ID to apply, or leave empty to remove'),
                        {
                            displayName: 'First Comment',
                            name: 'firstComment',
                            type: 'string',
                            typeOptions: { rows: 2 },
                            default: '',
                            description: 'Text to post as the first comment immediately after publishing',
                        },
                        ...platformSuffixFields('Account ID', 'AccountId', 'ID of the account to post from (leave empty for default)'),
                        ...platformContentOverrideFields(),
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
                            options: [...ALL_PLATFORMS_FILTER],
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
                // ── Account ─────────────────────────────────────────────────────
                if (resource === 'account') {
                    if (operation === 'list') {
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'GET',
                            url: `${baseUrl}/api/accounts`,
                        });
                    }
                    else {
                        throw new n8n_workflow_1.NodeOperationError(this.getNode(), `Unknown account operation: ${operation}`);
                    }
                    // ── AI Text ─────────────────────────────────────────────────────
                }
                else if (resource === 'aiText') {
                    if (operation === 'generate') {
                        const prompt = this.getNodeParameter('prompt', i);
                        const platforms = this.getNodeParameter('platforms', i, []);
                        const body = { prompt };
                        if (platforms.length)
                            body.platforms = platforms;
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'POST',
                            url: `${baseUrl}/api/generate-text`,
                            body,
                            headers: { 'Content-Type': 'application/json' },
                        });
                    }
                    else {
                        throw new n8n_workflow_1.NodeOperationError(this.getNode(), `Unknown aiText operation: ${operation}`);
                    }
                    // ── Mention ─────────────────────────────────────────────────────
                }
                else if (resource === 'mention') {
                    if (operation === 'list') {
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'GET',
                            url: `${baseUrl}/api/mentions`,
                        });
                    }
                    else if (operation === 'create') {
                        const name = this.getNodeParameter('name', i);
                        const extra = this.getNodeParameter('additionalFields', i, {});
                        const body = { name };
                        if (extra.country)
                            body.country = extra.country;
                        const handles = collectPlatformMap(extra, 'Handle', PLATFORM_KEYS);
                        if (Object.keys(handles).length)
                            body.handles = handles;
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'POST',
                            url: `${baseUrl}/api/mentions`,
                            body,
                            headers: { 'Content-Type': 'application/json' },
                        });
                    }
                    else if (operation === 'update') {
                        const mentionId = this.getNodeParameter('mentionId', i);
                        const fields = this.getNodeParameter('updateFields', i, {});
                        const body = {};
                        if (fields.name !== undefined && fields.name !== '')
                            body.name = fields.name;
                        if (fields.country !== undefined && fields.country !== '')
                            body.country = fields.country;
                        const handles = collectPlatformMap(fields, 'Handle', PLATFORM_KEYS);
                        if (Object.keys(handles).length)
                            body.handles = handles;
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'PUT',
                            url: `${baseUrl}/api/mentions/${mentionId}`,
                            body,
                            headers: { 'Content-Type': 'application/json' },
                        });
                    }
                    else if (operation === 'delete') {
                        const mentionId = this.getNodeParameter('mentionId', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'DELETE',
                            url: `${baseUrl}/api/mentions/${mentionId}`,
                        });
                    }
                    else if (operation === 'export') {
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'GET',
                            url: `${baseUrl}/api/mentions/export`,
                        });
                    }
                    else if (operation === 'import') {
                        const jsonData = this.getNodeParameter('jsonData', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'POST',
                            url: `${baseUrl}/api/mentions/import`,
                            body: JSON.parse(jsonData),
                            headers: { 'Content-Type': 'application/json' },
                        });
                    }
                    else {
                        throw new n8n_workflow_1.NodeOperationError(this.getNode(), `Unknown mention operation: ${operation}`);
                    }
                    // ── Post ────────────────────────────────────────────────────────
                }
                else if (resource === 'post') {
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
                        const suffixIds = collectPlatformMap(extra, 'SuffixId', PLATFORM_KEYS);
                        if (Object.keys(suffixIds).length)
                            body.suffixIds = suffixIds;
                        if (extra.firstComment)
                            body.firstComment = extra.firstComment;
                        if (extra.tags)
                            body.tags = parseList(extra.tags);
                        const accountIds = collectPlatformMap(extra, 'AccountId', PLATFORM_KEYS);
                        if (Object.keys(accountIds).length)
                            body.accountIds = accountIds;
                        const contentOverrides = collectPlatformMap(extra, 'ContentOverride', PLATFORM_KEYS);
                        if (Object.keys(contentOverrides).length)
                            body.contentOverrides = contentOverrides;
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
                        body.suffixIds = collectPlatformMap(fields, 'SuffixId', PLATFORM_KEYS);
                        if (fields.firstComment !== undefined && fields.firstComment !== '')
                            body.firstComment = fields.firstComment;
                        if (fields.tags)
                            body.tags = parseList(fields.tags);
                        const accountIds = collectPlatformMap(fields, 'AccountId', PLATFORM_KEYS);
                        if (Object.keys(accountIds).length)
                            body.accountIds = accountIds;
                        body.contentOverrides = collectPlatformMap(fields, 'ContentOverride', PLATFORM_KEYS);
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
                    else if (operation === 'retry') {
                        const postId = this.getNodeParameter('postId', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'POST',
                            url: `${baseUrl}/api/posts/${postId}/retry`,
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
                    // ── Watermark ───────────────────────────────────────────────────
                }
                else if (resource === 'watermark') {
                    if (operation === 'list') {
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'GET',
                            url: `${baseUrl}/api/watermarks`,
                        });
                    }
                    else if (operation === 'delete') {
                        const watermarkId = this.getNodeParameter('watermarkId', i);
                        responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
                            method: 'DELETE',
                            url: `${baseUrl}/api/watermarks/${watermarkId}`,
                        });
                    }
                    else {
                        throw new n8n_workflow_1.NodeOperationError(this.getNode(), `Unknown watermark operation: ${operation}`);
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
