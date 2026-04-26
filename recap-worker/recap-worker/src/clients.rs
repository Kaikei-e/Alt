pub(crate) mod alt_backend;
pub(crate) mod knowledge_sovereign;
pub(crate) mod mtls;
pub(crate) mod news_creator;
pub(crate) mod subworker;
pub(crate) mod tag_generator;
pub(crate) mod token_counter;

#[allow(unused_imports)]
pub(crate) use knowledge_sovereign::{KnowledgeSovereignClient, TopicSnapshottedInput};
pub(crate) use news_creator::NewsCreatorClient;
pub(crate) use subworker::SubworkerClient;
pub(crate) use tag_generator::TagGeneratorClient;
pub(crate) use token_counter::TokenCounter;

#[cfg(test)]
mod alt_backend_contract;
#[cfg(test)]
mod subworker_contract;
#[cfg(test)]
mod tag_generator_contract;
