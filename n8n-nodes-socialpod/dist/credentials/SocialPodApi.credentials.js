"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SocialPodApi = void 0;
class SocialPodApi {
    constructor() {
        this.name = 'socialPodApi';
        this.displayName = 'SocialPod API';
        this.documentationUrl = 'https://github.com/michaelkleinhenz/socialpod';
        this.properties = [
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
                description: 'API token from your SocialPod profile page (sm_... for users, st_... for teams)',
            },
        ];
        this.authenticate = {
            type: 'generic',
            properties: {
                headers: {
                    Authorization: '={{"Bearer " + $credentials.apiToken}}',
                },
            },
        };
        this.test = {
            request: {
                baseURL: '={{$credentials.apiUrl}}',
                url: '/api/auth/me',
            },
        };
    }
}
exports.SocialPodApi = SocialPodApi;
