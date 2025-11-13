use recap_worker::evaluation::genre::{
    EvaluationSettings, GenreEvaluationCandidate, GenreEvaluationGraphEdge, GenreEvaluationSample,
    GenreEvaluationTag, evaluate_two_stage,
};
use uuid::Uuid;

#[tokio::test]
async fn two_stage_evaluation_highlights_graph_boost() {
    let samples = vec![
        tech_with_tag(),
        graph_boost_example(),
        tagless_health(),
        business_with_tag(),
    ];

    let report = evaluate_two_stage(&samples, EvaluationSettings::default())
        .await
        .expect("evaluation should complete");

    assert!(report.two_stage.accuracy > report.coarse.accuracy);
    assert!(report.two_stage.macro_f1 > report.coarse.macro_f1);
    assert!(report.two_stage.macro_precision >= report.tag.macro_precision);
    assert_eq!(report.two_stage.accuracy, 1.0);
}

fn tech_with_tag() -> GenreEvaluationSample {
    let mut sample = GenreEvaluationSample::with_defaults(
        Uuid::new_v4(),
        "article-tech-1",
        "tech",
        vec![
            GenreEvaluationCandidate {
                genre: "tech".into(),
                score: 0.92,
                keyword_support: 4,
                classifier_confidence: 0.89,
            },
            GenreEvaluationCandidate {
                genre: "business".into(),
                score: 0.78,
                keyword_support: 2,
                classifier_confidence: 0.75,
            },
        ],
    );
    sample.tags = vec![GenreEvaluationTag {
        label: "tech".into(),
        confidence: 0.88,
    }];
    sample.sentences = vec!["machine learning accelerates product design".into()];
    sample.language = "en".into();
    sample
}

fn graph_boost_example() -> GenreEvaluationSample {
    let mut sample = GenreEvaluationSample::with_defaults(
        Uuid::new_v4(),
        "article-tech-2",
        "tech",
        vec![
            GenreEvaluationCandidate {
                genre: "business".into(),
                score: 0.85,
                keyword_support: 2,
                classifier_confidence: 0.83,
            },
            GenreEvaluationCandidate {
                genre: "tech".into(),
                score: 0.83,
                keyword_support: 3,
                classifier_confidence: 0.81,
            },
        ],
    );
    sample.tags = vec![GenreEvaluationTag {
        label: "半導体".into(),
        confidence: 0.75,
    }];
    sample.graph_edges = vec![
        GenreEvaluationGraphEdge {
            genre: "tech".into(),
            tag: "半導体".into(),
            weight: 0.28,
        },
        GenreEvaluationGraphEdge {
            genre: "business".into(),
            tag: "半導体".into(),
            weight: 0.05,
        },
    ];
    sample.sentences = vec!["chip funding and fabs dominate the week".into()];
    sample.language = "ja".into();
    sample
}

fn tagless_health() -> GenreEvaluationSample {
    let mut sample = GenreEvaluationSample::with_defaults(
        Uuid::new_v4(),
        "article-health-1",
        "health",
        vec![
            GenreEvaluationCandidate {
                genre: "health".into(),
                score: 0.91,
                keyword_support: 3,
                classifier_confidence: 0.87,
            },
            GenreEvaluationCandidate {
                genre: "science".into(),
                score: 0.86,
                keyword_support: 1,
                classifier_confidence: 0.84,
            },
        ],
    );
    sample.sentences = vec!["healthcare policy updates across regions".into()];
    sample.language = "en".into();
    sample
}

fn business_with_tag() -> GenreEvaluationSample {
    let mut sample = GenreEvaluationSample::with_defaults(
        Uuid::new_v4(),
        "article-business-1",
        "business",
        vec![
            GenreEvaluationCandidate {
                genre: "business".into(),
                score: 0.88,
                keyword_support: 5,
                classifier_confidence: 0.86,
            },
            GenreEvaluationCandidate {
                genre: "world".into(),
                score: 0.65,
                keyword_support: 1,
                classifier_confidence: 0.62,
            },
        ],
    );
    sample.tags = vec![GenreEvaluationTag {
        label: "business".into(),
        confidence: 0.78,
    }];
    sample.sentences = vec!["market rallies follow central bank guidance".into()];
    sample
}
