
import streamlit as st
from utils import fetch_metrics

def render_summarization():
    st.header("Summarization Metrics")
    df_sum = fetch_metrics("summarization")
    if not df_sum.empty:
        st.subheader("Performance Metrics")
        expected_cols = ['json_validation_errors', 'summary_length_bullets', 'processing_time_ms', 'relevance', 'faithfulness']
        for col in expected_cols:
            if col not in df_sum.columns:
                df_sum[col] = 0

        st.line_chart(df_sum, x="timestamp", y="processing_time_ms")

        col1, col2, col3, col4 = st.columns(4)
        latest = df_sum.iloc[0]
        col1.metric("JSON Errors", int(latest['json_validation_errors']))
        col2.metric("Length", int(latest['summary_length_bullets']))
        col3.metric("Time (ms)", int(latest['processing_time_ms']))
        col4.metric("Faithfulness", f"{float(latest['faithfulness']):.2f}")

        st.subheader("Error Rate")
        st.bar_chart(df_sum, x="timestamp", y="json_validation_errors")
    else:
        st.info("No summarization metrics found.")
