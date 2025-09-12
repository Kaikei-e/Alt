export default function DesktopLoading() {
  return (
    <div
      style={{
        padding: 24,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "100vh",
        backgroundColor: "var(--app-bg)",
      }}
    >
      <div
        style={{
          width: 40,
          height: 40,
          border: "4px solid #e2e8f0",
          borderTop: "4px solid #3182ce",
          borderRadius: "50%",
          marginBottom: 16,
        }}
      />
      <p style={{ color: "#718096" }}>デスクトップページを読み込み中...</p>
    </div>
  );
}
