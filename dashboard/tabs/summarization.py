
import streamlit as st
from utils import fetch_metrics

def render_summarization():
    st.header("Summarization Metrics")
    df_sum = fetch_metrics("summarization")

    if df_sum.empty:
        st.info("No summarization metrics found.")
        return

    # Ensure expected columns
    expected_cols = [
        'json_validation_errors',
        'summary_length_bullets',
        'processing_time_ms',
        'faithfulness',  # Also referred to as Alignment
        'coverage_score',
        'mmr_diversity'
    ]
    for col in expected_cols:
        if col not in df_sum.columns:
            df_sum[col] = 0.0

    # Key Quality Indicators
    st.subheader("Quality & Reliability (Latest)")
    latest = df_sum.iloc[0]

    col1, col2, col3, col4 = st.columns(4)
    col1.metric("Alignment (Faithfulness)", f"{float(latest['faithfulness']):.2f}", help="Semantic alignment with source text.")
    col2.metric("Coverage Score", f"{float(latest['coverage_score']):.2f}", help="Information coverage of the summary.")
    col3.metric("JSON Errors", int(latest['json_validation_errors']), delta_color="inverse")
    col4.metric("MMR Diversity", f"{float(latest['mmr_diversity']):.2f}")

    # Performance Trends
    st.subheader("Performance Trends")
    tab1, tab2 = st.tabs(["Processing Time", "Quality Scores"])

    with tab1:
        st.line_chart(df_sum.set_index("timestamp")["processing_time_ms"])
        st.caption("Processing time in milliseconds.")

    with tab2:
        st.line_chart(df_sum.set_index("timestamp")[["faithfulness", "coverage_score"]])
        st.caption("Faithfulness (Alignment) and Coverage scores over time.")

    # Output Statistics
    st.subheader("Output Statistics (Latest)")
    col1, col2 = st.columns(2)
    col1.metric("Summary Length (Bullets)", int(latest['summary_length_bullets']))
    # If we had 'input_sentences_count' we could show compression ratio

