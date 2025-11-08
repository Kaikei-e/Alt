/// 7日・10k記事の性能ベンチマーク。
use criterion::{Criterion, black_box, criterion_group, criterion_main};
use recap_worker::analysis::{keyword_scores, preprocess_documents, synthetic_bodies};

fn bench_preprocessing(c: &mut Criterion) {
    let bodies = synthetic_bodies(1024, 6);
    c.bench_function("preprocess_articles_1k", |b| {
        b.iter(|| {
            let (count, processed) = preprocess_documents(&bodies);
            black_box((count, processed.len()));
        });
    });
}

fn bench_keyword_scoring(c: &mut Criterion) {
    let bodies = synthetic_bodies(512, 5);
    let (_, processed) = preprocess_documents(&bodies);

    c.bench_function("keyword_scoring_500_docs", |b| {
        b.iter(|| {
            let scores = keyword_scores(&processed);
            black_box(scores.len());
        });
    });
}

criterion_group!(benches, bench_preprocessing, bench_keyword_scoring);
criterion_main!(benches);
