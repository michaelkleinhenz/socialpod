import {
  IDataObject,
  IExecuteFunctions,
  INodeExecutionData,
  INodeType,
  INodeTypeDescription,
  NodeOperationError,
} from 'n8n-workflow';
import FormData from 'form-data';

// Sends a multipart/form-data request with post data in the `data` JSON field.
async function multipartRequest(
  ctx: IExecuteFunctions,
  method: 'POST' | 'PUT',
  url: string,
  data: IDataObject,
): Promise<IDataObject> {
  const form = new FormData();
  form.append('data', JSON.stringify(data));
  return ctx.helpers.httpRequestWithAuthentication.call(ctx, 'socialPodApi', {
    method,
    url,
    body: form,
    headers: form.getHeaders(),
  }) as Promise<IDataObject>;
}

// Parses a comma-separated string into a trimmed, non-empty string array.
function parseList(value: string): string[] {
  return value.split(',').map((s) => s.trim()).filter(Boolean);
}

export class SocialPod implements INodeType {
  description: INodeTypeDescription = {
    displayName: 'SocialPod',
    name: 'socialPod',
    icon: 'file:socialpod.svg',
    group: ['output'],
    version: 1,
    subtitle: '={{$parameter["operation"] + ": " + $parameter["resource"]}}',
    description: 'Schedule and manage social media posts via SocialPod',
    defaults: { name: 'SocialPod' },
    inputs: ['main'] as any,
    outputs: ['main'] as any,
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
          { name: 'Create',     value: 'create',     action: 'Create a post' },
          { name: 'Delete',     value: 'delete',     action: 'Delete a post' },
          { name: 'Get',        value: 'get',        action: 'Get a post by ID' },
          { name: 'List',       value: 'list',       action: 'List posts' },
          { name: 'Reschedule', value: 'reschedule', action: 'Reschedule a post' },
          { name: 'Update',     value: 'update',     action: 'Update a post' },
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
          { name: 'List',   value: 'list',   action: 'List suffixes' },
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
          { name: 'Bluesky',   value: 'bluesky' },
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
        displayName: 'Status',
        name: 'status',
        type: 'options',
        displayOptions: { show: { resource: ['post'], operation: ['create'] } },
        options: [
          { name: 'Scheduled', value: 'scheduled' },
          { name: 'Draft',     value: 'draft' },
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
            displayName: 'Tags',
            name: 'tags',
            type: 'string',
            default: '',
            placeholder: 'news, automation, tech',
            description: 'Comma-separated list of tags (without #)',
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
            displayName: 'Platforms',
            name: 'platforms',
            type: 'multiOptions',
            options: [
              { name: 'Bluesky',   value: 'bluesky' },
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
              { name: 'Draft',     value: 'draft' },
            ],
            default: 'scheduled',
          },
          {
            displayName: 'Tags',
            name: 'tags',
            type: 'string',
            default: '',
            placeholder: 'news, automation, tech',
            description: 'Comma-separated list of tags (replaces existing tags)',
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
              { name: 'Any',       value: '' },
              { name: 'Draft',     value: 'draft' },
              { name: 'Failed',    value: 'failed' },
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
              { name: 'Any',       value: '' },
              { name: 'Bluesky',   value: 'bluesky' },
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

  async execute(this: IExecuteFunctions): Promise<INodeExecutionData[][]> {
    const items = this.getInputData();
    const returnData: INodeExecutionData[] = [];

    const credentials = await this.getCredentials('socialPodApi');
    const baseUrl = (credentials.apiUrl as string).replace(/\/$/, '');

    for (let i = 0; i < items.length; i++) {
      try {
        const resource  = this.getNodeParameter('resource',  i) as string;
        const operation = this.getNodeParameter('operation', i) as string;
        let responseData: IDataObject | IDataObject[];

        // ── Post ────────────────────────────────────────────────────────
        if (resource === 'post') {

          if (operation === 'create') {
            const content     = this.getNodeParameter('content',     i) as string;
            const platforms   = this.getNodeParameter('platforms',   i) as string[];
            const scheduledAt = this.getNodeParameter('scheduledAt', i) as string;
            const status      = this.getNodeParameter('status',      i) as string;
            const extra       = this.getNodeParameter('additionalFields', i, {}) as IDataObject;

            const body: IDataObject = { content, platforms, scheduledAt, status };
            if (extra.tags)      body.tags      = parseList(extra.tags as string);
            if (extra.imageUrls) body.imageUrls = parseList(extra.imageUrls as string);

            const suffixIds: IDataObject = {};
            if (extra.bluskySuffixId)    suffixIds.bluesky   = extra.bluskySuffixId;
            if (extra.instagramSuffixId) suffixIds.instagram = extra.instagramSuffixId;
            if (Object.keys(suffixIds).length) body.suffixIds = suffixIds;

            responseData = await multipartRequest(this, 'POST', `${baseUrl}/api/posts`, body);

          } else if (operation === 'get') {
            const postId = this.getNodeParameter('postId', i) as string;
            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'GET',
              url: `${baseUrl}/api/posts/${postId}`,
            }) as IDataObject;

          } else if (operation === 'list') {
            const filters = this.getNodeParameter('filters', i, {}) as IDataObject;
            const qs: IDataObject = {};
            if (filters.start)    qs.start    = filters.start;
            if (filters.end)      qs.end      = filters.end;
            if (filters.status)   qs.status   = filters.status;
            if (filters.platform) qs.platform = filters.platform;

            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'GET',
              url: `${baseUrl}/api/posts`,
              qs,
            }) as IDataObject[];

          } else if (operation === 'update') {
            const postId = this.getNodeParameter('postId', i) as string;
            const fields = this.getNodeParameter('updateFields', i, {}) as IDataObject;

            const body: IDataObject = {};
            if (fields.content     !== undefined && fields.content     !== '') body.content     = fields.content;
            if (fields.scheduledAt !== undefined && fields.scheduledAt !== '') body.scheduledAt = fields.scheduledAt;
            if (fields.status      !== undefined && fields.status      !== '') body.status      = fields.status;
            if (fields.tags        !== undefined && fields.tags        !== '') body.tags        = parseList(fields.tags as string);
            if (fields.imageUrls   !== undefined && fields.imageUrls   !== '') body.imageUrls   = parseList(fields.imageUrls as string);
            if ((fields.platforms  as string[] | undefined)?.length)           body.platforms   = fields.platforms;

            const suffixIds: IDataObject = {};
            if (fields.bluskySuffixId)    suffixIds.bluesky   = fields.bluskySuffixId;
            if (fields.instagramSuffixId) suffixIds.instagram = fields.instagramSuffixId;
            body.suffixIds = suffixIds; // always send — empty map clears suffixes

            responseData = await multipartRequest(this, 'PUT', `${baseUrl}/api/posts/${postId}`, body);

          } else if (operation === 'delete') {
            const postId = this.getNodeParameter('postId', i) as string;
            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'DELETE',
              url: `${baseUrl}/api/posts/${postId}`,
            }) as IDataObject;

          } else if (operation === 'reschedule') {
            const postId      = this.getNodeParameter('postId',      i) as string;
            const scheduledAt = this.getNodeParameter('scheduledAt', i) as string;
            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'PATCH',
              url: `${baseUrl}/api/posts/${postId}/reschedule`,
              body: { scheduledAt },
              headers: { 'Content-Type': 'application/json' },
            }) as IDataObject;

          } else {
            throw new NodeOperationError(this.getNode(), `Unknown post operation: ${operation}`);
          }

        // ── Suffix ──────────────────────────────────────────────────────
        } else if (resource === 'suffix') {

          if (operation === 'list') {
            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'GET',
              url: `${baseUrl}/api/suffixes`,
            }) as IDataObject[];

          } else if (operation === 'create') {
            const name    = this.getNodeParameter('name',    i) as string;
            const content = this.getNodeParameter('content', i) as string;
            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'POST',
              url: `${baseUrl}/api/suffixes`,
              body: { name, content },
              headers: { 'Content-Type': 'application/json' },
            }) as IDataObject;

          } else if (operation === 'update') {
            const suffixId = this.getNodeParameter('suffixId', i) as string;
            const fields   = this.getNodeParameter('updateFields', i, {}) as IDataObject;
            const body: IDataObject = {};
            if (fields.name    !== undefined && fields.name    !== '') body.name    = fields.name;
            if (fields.content !== undefined && fields.content !== '') body.content = fields.content;
            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'PUT',
              url: `${baseUrl}/api/suffixes/${suffixId}`,
              body,
              headers: { 'Content-Type': 'application/json' },
            }) as IDataObject;

          } else if (operation === 'delete') {
            const suffixId = this.getNodeParameter('suffixId', i) as string;
            responseData = await this.helpers.httpRequestWithAuthentication.call(this, 'socialPodApi', {
              method: 'DELETE',
              url: `${baseUrl}/api/suffixes/${suffixId}`,
            }) as IDataObject;

          } else {
            throw new NodeOperationError(this.getNode(), `Unknown suffix operation: ${operation}`);
          }

        } else {
          throw new NodeOperationError(this.getNode(), `Unknown resource: ${resource}`);
        }

        const rows = Array.isArray(responseData) ? responseData : [responseData];
        returnData.push(...rows.map((json) => ({ json, pairedItem: { item: i } })));

      } catch (error) {
        if (this.continueOnFail()) {
          returnData.push({
            json: { error: (error as Error).message },
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
