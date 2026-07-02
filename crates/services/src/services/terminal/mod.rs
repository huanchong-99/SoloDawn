//! Terminal management module
//!
//! Provides:
//! - ProcessManager: Process lifecycle management (spawn, kill, monitor)
//! - CliDetector: CLI availability and version detection
//! - TerminalLauncher: Serial terminal launcher with model switching
//! - TerminalBridge: MessageBus -> PTY stdin bridge for Orchestrator communication
//! - PromptDetector: Interactive prompt detection and classification
//! - PromptWatcher: PTY output monitoring and prompt event publishing
//! - OutputFanout: Single-reader PTY output fanout with replay support

pub mod bridge;
pub mod detector;
pub mod launcher;
pub mod output_fanout;
pub mod process;
pub mod prompt_detector;
pub mod prompt_watcher;
pub mod utf8_decoder;

pub use bridge::TerminalBridge;
pub use detector::CliDetector;
pub use launcher::{LaunchResult, TerminalLauncher, cli_binary_name};
pub use output_fanout::{OutputChunk, OutputFanout, OutputFanoutConfig, OutputSubscription};
pub use process::{ProcessHandle, ProcessManager};
pub use prompt_detector::{
    ARROW_DOWN, ARROW_UP, ArrowSelectOption, DetectedPrompt, PromptDetector, PromptKind,
    build_arrow_sequence,
};
pub use prompt_watcher::PromptWatcher;
pub use utf8_decoder::{Utf8DecodeChunk, Utf8DecodeStats, Utf8StreamDecoder};
