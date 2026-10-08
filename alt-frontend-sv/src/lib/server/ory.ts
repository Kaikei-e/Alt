import { Configuration, FrontendApi } from "@ory/client";
import { KRATOS_INTERNAL_URL } from "$app/env/private";

const kratosInternalUrl = KRATOS_INTERNAL_URL || "http://kratos:4433";

export const ory = new FrontendApi(
	new Configuration({
		basePath: kratosInternalUrl,
		baseOptions: {
			withCredentials: true,
		},
	}),
);
