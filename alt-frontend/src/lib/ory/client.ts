import { Configuration, FrontendApi } from "@ory/client";
import { KRATOS_PUBLIC_URL } from "@/lib/env.public";

const configuration = new Configuration({
  basePath: KRATOS_PUBLIC_URL,
  baseOptions: {
    withCredentials: true,
  },
});

export const oryClient = new FrontendApi(configuration);
