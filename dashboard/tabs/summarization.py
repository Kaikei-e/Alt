
import streamlit as st
import pandas as pd
from utils import fetch_metrics


def safe_int(value, default=0):
    """Safely convert a value to integer, handling NaN and None."""
    if pd.isna(value) or value is None:
        return default
    try:
        return int(float(value))
    except (ValueError, TypeError):
        return default


def safe_float(value, default=0.0):
    """Safely convert a value to float, handling NaN and None."""
    if pd.isna(value) or value is None:
        return default
    try:
        return float(value)
    except (ValueError, TypeError):
        return default


def render_summarization(window_seconds: int):
    st.header("Summarization Metrics")
    df_sum = fetch_metrics("summarization", window_seconds)

    if df_sum.empty:
        st.info("No summarization metrics found.")
        return

    # Ensure expected columns and handle NaN values
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
        else:
            # Fill NaN values with appropriate defaults
            if col in ['json_validation_errors', 'summary_length_bullets']:
                df_sum[col] = df_sum[col].fillna(0).astype(float)
            else:
                df_sum[col] = df_sum[col].fillna(0.0)

    # Key Quality Indicators
    st.subheader("Quality & Reliability (Latest)")
    latest = df_sum.iloc[0]

    col1, col2, col3, col4 = st.columns(4)
    col1.metric("Alignment (Faithfulness)", f"{safe_float(latest.get('faithfulness', 0.0)):.2f}", help="Semantic alignment with source text.")
    col2.metric("Coverage Score", f"{safe_float(latest.get('coverage_score', 0.0)):.2f}", help="Information coverage of the summary.")
    col3.metric("JSON Errors", safe_int(latest.get('json_validation_errors', 0)), delta_color="inverse")
    col4.metric("MMR Diversity", f"{safe_float(latest.get('mmr_diversity', 0.0)):.2f}")

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
    col1.metric("Summary Length (Bullets)", safe_int(latest.get('summary_length_bullets', 0)))
    # If we had 'input_sentences_count' we could show compression ratio

