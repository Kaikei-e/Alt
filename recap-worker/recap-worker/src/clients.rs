pub(crate) mod alt_backend;
pub(crate) mod news_creator;
pub(crate) mod subworker;
pub(crate) mod tag_generator;

pub(crate) use news_creator::NewsCreatorClient;
pub(crate) use subworker::SubworkerClient;
pub(crate) use tag_generator::TagGeneratorClient;
