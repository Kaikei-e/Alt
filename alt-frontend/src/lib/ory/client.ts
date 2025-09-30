import { Configuration, FrontendApi } from "@ory/client";
import { KRATOS_PUBLIC_URL } from "@/lib/env.public";

const configuration = new Configuration({
  basePath: KRATOS_PUBLIC_URL,
  baseOptions: {
    // withCredentials is not needed since /ory/ is proxied on the same origin
    // Cookies are automatically included for same-origin requests
    timeout: 10000, // 10 second timeout
    headers: {
      Accept: "application/json",
    },
  },
});

export const oryClient = new FrontendApi(configuration);
