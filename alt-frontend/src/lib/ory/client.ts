import { Configuration, FrontendApi } from "@ory/client";
import { KRATOS_PUBLIC_URL } from "@/lib/env.public";

const configuration = new Configuration({
  basePath: KRATOS_PUBLIC_URL,
  baseOptions: {
    // withCredentials is needed for cross-origin requests (e.g., in tests)
    // For same-origin /ory/ proxy, this is also safe to enable
    withCredentials: true,
    timeout: 10000, // 10 second timeout
    headers: {
      Accept: "application/json",
    },
  },
});

export const oryClient = new FrontendApi(configuration);
