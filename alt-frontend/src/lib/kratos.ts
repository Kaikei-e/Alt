import { Configuration, FrontendApi } from '@ory/client';

const kratosConfig = new Configuration({
  basePath: '/ory',
  baseOptions: {
    credentials: 'include'
  }
});

export const kratos = new FrontendApi(kratosConfig);