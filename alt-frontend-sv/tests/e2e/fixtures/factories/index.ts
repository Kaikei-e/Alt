/**
 * Mock data factories - single entry point for all test data builders.
 */

// Feed factories
export {
	buildFeedV1,
	buildFeedsV1Response,
	buildConnectFeedItem,
	buildConnectFeedsResponse,
	buildConnectArticleContent,
	resetFeedCounter,
	type FeedV1,
	type ConnectFeedItem,
} from "./feedFactory";

// Recap & Augur factories
export {
	buildRecapGenre,
	buildEvidenceLink,
	buildConnectRecapResponse,
	buildAugurStreamMessages,
	buildMorningLetterStreamMessages,
	type RecapGenre,
	type EvidenceLink,
} from "./recapFactory";

// Session & Auth factories
export {
	DEV_USER_ID,
	DEV_JWT_SECRET,
	KRATOS_SESSION_COOKIE_NAME,
	KRATOS_SESSION_COOKIE_VALUE,
	buildKratosSession,
	buildLoginFlow,
	buildRegistrationFlow,
	buildAuthHubSession,
} from "./sessionFactory";
