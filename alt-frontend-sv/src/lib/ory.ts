import { Configuration, FrontendApi } from "@ory/client";
import { env } from "$env/dynamic/private";

// Kratos内部URL（サーバーサイドからのアクセス用）
const kratosInternalUrl = env.KRATOS_INTERNAL_URL || "http://kratos:4433";

export const ory = new FrontendApi(
  new Configuration({
    basePath: kratosInternalUrl,
    baseOptions: {
      withCredentials: true,
    },
  }),
);
