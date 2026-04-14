use std::{future::Future, sync::OnceLock};

use tokio::runtime::{Builder, Handle, Runtime, RuntimeFlavor};

// [W2-36-07] The global Runtime stored in this OnceLock is intentionally
// never dropped. It is a process-wide executor used by `block_on` when sync
// code needs to drive async work, so its lifetime is the program lifetime.
// The OS reclaims all associated threads and file descriptors on process
// exit; running `Runtime::shutdown_*` at exit would offer no benefit and
// could deadlock if called from within an outstanding `block_on`.
fn rt() -> &'static Runtime {
    static RT: OnceLock<Runtime> = OnceLock::new();
    RT.get_or_init(|| {
        Builder::new_multi_thread()
            .enable_all()
            .build()
            .expect("failed to build global Tokio runtime")
    })
}

/// Run an async future from sync code safely.
/// If already inside a Tokio runtime, it will use that runtime.
pub fn block_on<F, T>(fut: F) -> T
where
    F: Future<Output = T> + Send,
    T: Send,
{
    match Handle::try_current() {
        // Already inside a Tokio runtime
        Ok(h) => match h.runtime_flavor() {
            // Use block_in_place so other tasks keep running.
            RuntimeFlavor::MultiThread => tokio::task::block_in_place(|| rt().block_on(fut)),
            // Spawn a new thread to avoid freezing a single-thread runtime.
            RuntimeFlavor::CurrentThread | _ => std::thread::scope(|s| {
                s.spawn(|| rt().block_on(fut))
                    .join()
                    .expect("thread panicked")
            }),
        },
        // Outside Tokio: block normally.
        Err(_) => rt().block_on(fut),
    }
}
