/**
 * Mock data factories - single entry point for all test data builders.
 */

// Feed factories
export {
	buildConnectArticleContent,
	buildConnectFeedItem,
	buildConnectFeedsResponse,
	buildFeedsV1Response,
	buildFeedV1,
	type ConnectFeedItem,
	type FeedV1,
	resetFeedCounter,
} from "./feedFactory";
// Pulse factories
export {
	buildEveningPulseResponse,
	buildPulseTopic,
	buildQuietDayResponse,
	type EveningPulseResponse,
	type PulseTopic,
	resetPulseCounter,
} from "./pulseFactory";
// Recap & Augur factories
export {
	buildAugurStreamMessages,
	buildConnectRecapResponse,
	buildEvidenceLink,
	buildMorningLetterStreamMessages,
	buildRecapGenre,
	type EvidenceLink,
	type RecapGenre,
} from "./recapFactory";
// Session & Auth factories
export {
	buildAuthHubSession,
	buildKratosSession,
	buildLoginFlow,
	buildRegistrationFlow,
	DEV_JWT_SECRET,
	DEV_USER_ID,
	KRATOS_SESSION_COOKIE_NAME,
	KRATOS_SESSION_COOKIE_VALUE,
} from "./sessionFactory";

// Tag Trail factories
export {
	buildArticlesByTagResponse,
	buildTagStreamMessages,
	buildTagTrailArticle,
	buildTagTrailFeed,
	resetTrailCounter,
	type TagTrailArticle,
	type TagTrailFeed,
} from "./tagTrailFactory";
