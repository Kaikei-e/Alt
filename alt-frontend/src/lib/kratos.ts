import { Configuration, FrontendApi } from '@ory/client';

const kratosConfig = new Configuration({
  basePath: process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL,
  baseOptions: {
    withCredentials: true,
  }
});

export const kratos = new FrontendApi(kratosConfig);