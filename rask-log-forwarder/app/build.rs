fn main() {
    // For now, just rebuild if Cargo.toml changes
    println!("cargo:rerun-if-changed=Cargo.toml");

    // In a full implementation, we would use vergen to embed git info:
    // use vergen::EmitBuilder;
    // EmitBuilder::builder()
    //     .git_sha(false)
    //     .git_describe(true, true, None)
    //     .emit().expect("Unable to generate version information");
}
