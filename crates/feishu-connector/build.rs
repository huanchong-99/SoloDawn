fn main() -> Result<(), Box<dyn std::error::Error>> {
    prost_build::compile_protos(&["proto/pbbp2.proto"], &["proto"])?;
    Ok(())
}
