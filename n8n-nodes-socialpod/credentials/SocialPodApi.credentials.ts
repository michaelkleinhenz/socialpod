import {
  IAuthenticateGeneric,
  ICredentialTestRequest,
  ICredentialType,
  INodeProperties,
} from 'n8n-workflow';

export class SocialPodApi implements ICredentialType {
  name = 'socialPodApi';
  displayName = 'SocialPod API';
  documentationUrl = 'https://github.com/michaelkleinhenz/socialpod';

  properties: INodeProperties[] = [
    {
      displayName: 'API URL',
      name: 'apiUrl',
      type: 'string',
      default: 'http://localhost:8080',
      placeholder: 'https://socialpod.example.com',
      description: 'Base URL of your SocialPod instance (no trailing slash)',
    },
    {
      displayName: 'API Token',
      name: 'apiToken',
      type: 'string',
      typeOptions: { password: true },
      default: '',
      description:
        'API token from your SocialPod profile page (sm_... for users, st_... for teams)',
    },
  ];

  authenticate: IAuthenticateGeneric = {
    type: 'generic',
    properties: {
      headers: {
        Authorization: '={{"Bearer " + $credentials.apiToken}}',
      },
    },
  };

  test: ICredentialTestRequest = {
    request: {
      baseURL: '={{$credentials.apiUrl}}',
      url: '/api/auth/me',
    },
  };
}
