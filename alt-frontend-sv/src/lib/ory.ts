import { Configuration, FrontendApi } from "@ory/client";

export const ory = new FrontendApi(
  new Configuration({
    basePath: "http://kratos:4433",
    baseOptions: {
      withCredentials: true,
    },
  }),
);
