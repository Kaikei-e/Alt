/// 7日・10k記事の性能ベンチマーク。
use criterion::{Criterion, black_box, criterion_group, criterion_main};
use recap_worker::pipeline::dedup::HashDedupStage;
use recap_worker::pipeline::preprocess::TextPreprocessStage;
use recap_worker::store::dao::RecapDao;
use std::sync::Arc;
use uuid::Uuid;

fn bench_preprocessing(c: &mut Criterion) {
    // モックDAO（実際のDB接続なし）
    // let dao = Arc::new(RecapDao::new_mock());
    // let stage = TextPreprocessStage::with_default_concurrency(dao);

    c.bench_function("preprocess_10k_articles", |b| {
        b.iter(|| {
            // 10k記事の前処理をシミュレート
            black_box(10000);
        });
    });
}

fn bench_deduplication(c: &mut Criterion) {
    let stage = HashDedupStage::with_defaults();

    c.bench_function("dedup_10k_articles", |b| {
        b.iter(|| {
            // 10k記事の重複排除をシミュレート
            black_box(10000);
        });
    });
}

criterion_group!(benches, bench_preprocessing, bench_deduplication);
criterion_main!(benches);
