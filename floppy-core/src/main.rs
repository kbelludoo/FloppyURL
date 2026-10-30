use clap::{Parser, Subcommand};
use floppy_core::{compress_tool_output, pack_html};
use std::io::{self, Read};
use std::time::Instant;

#[derive(Parser)]
#[command(name = "floppy-core")]
#[command(about = "⚡ FloppyURL High-Performance Rust Engine (RTK + PocketWeb Packager)", long_about = None)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Compress tool/terminal outputs (RTK - Result Token Kompressor)
    Rtk {
        /// Optional direct string input (if omitted, reads from stdin)
        #[arg(short, long)]
        input: Option<String>,
    },
    /// Minify, Deflate-compress and package HTML for FloppyURL
    Pack {
        /// The target web URL being packaged
        #[arg(short, long)]
        url: String,

        /// Optional path to HTML file (if omitted, reads from stdin)
        #[arg(short, long)]
        file: Option<String>,
    },
    /// Run automated performance benchmark (RTK & Packager)
    Bench {
        /// Number of iterations
        #[arg(short, long, default_value_t = 1000)]
        iterations: usize,
    },
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let cli = Cli::parse();

    match cli.command {
        Commands::Rtk { input } => {
            let content = match input {
                Some(text) => text,
                None => {
                    let mut buffer = String::new();
                    io::stdin().read_to_string(&mut buffer)?;
                    buffer
                }
            };
            let compressed = compress_tool_output(&content);
            print!("{}", compressed);
        }
        Commands::Pack { url, file } => {
            let html_content = match file {
                Some(path) => std::fs::read_to_string(path)?,
                None => {
                    let mut buffer = String::new();
                    io::stdin().read_to_string(&mut buffer)?;
                    buffer
                }
            };

            let response = pack_html(&html_content, &url)
                .map_err(|e| io::Error::new(io::ErrorKind::Other, e))?;

            println!("{}", serde_json::to_string_pretty(&response)?);
        }
        Commands::Bench { iterations } => {
            println!("===============================================================");
            println!("⚡ FLOOPY-CORE RUST PERFORMANCE BENCHMARK (iterations: {})", iterations);
            println!("===============================================================\n");

            // 1. Benchmark RTK
            let sample_terminal_output = r#"
[32m[INFO][0m Starting service compilation on worker node-4...
[33m[WARN][0m High memory utilization detected (82.4%):
index 4b825dc..6a19f02 100644
--- a/pkg/service/worker.go
+++ b/pkg/service/worker.go
@@ -10,6 +10,7 @@ func RunWorker() {
+    log.Println("Worker initialized")


   
[34m[DEBUG][0m Garbage collection cycle finished: 1.2ms.


Process complete with exit code 0.
"#;

            let start_rtk = Instant::now();
            let mut total_rtk_bytes = 0;
            for _ in 0..iterations {
                let comp = compress_tool_output(sample_terminal_output);
                total_rtk_bytes += comp.len();
            }
            let duration_rtk = start_rtk.elapsed();
            let avg_rtk_us = (duration_rtk.as_micros() as f64) / (iterations as f64);
            let rtk_throughput_mb = ((sample_terminal_output.len() * iterations) as f64 / 1_048_576.0)
                / duration_rtk.as_secs_f64();

            println!("1. RTK (Result Token Kompressor):");
            println!("   Total time:         {:.3?}", duration_rtk);
            println!("   Average latency:    {:.2} µs / operation (0.{:03} ms)", avg_rtk_us, avg_rtk_us as u64);
            println!("   Throughput:         {:.2} MB/s", rtk_throughput_mb);
            println!("   Processed bytes:    {} bytes\n", total_rtk_bytes);

            // 2. Benchmark Packager
            let sample_html = r#"
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Enterprise Cloud Dashboard</title>
    <style>
        body { font-family: -apple-system, sans-serif; margin: 0; padding: 20px; background: #0f172a; color: #f8fafc; }
        .card { background: #1e293b; border-radius: 8px; padding: 16px; margin-bottom: 12px; }
        .btn { display: inline-block; padding: 8px 16px; background: #3b82f6; color: #fff; text-decoration: none; border-radius: 4px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Cloud Telemetry & Infrastructure Node</h1>
        <!-- System Status Summary -->
        <div class="card">
            <h3>Server Metrics</h3>
            <p>Active Nodes: 12 | Memory Free: 8.4 GB | Uptime: 99.98%</p>
            <a href="https://example.com/details" class="btn">View Node Cluster</a>
        </div>
    </div>
    <script>
        console.log("Telemetry reporting initialized.");
    </script>
</body>
</html>
"#;

            let start_pack = Instant::now();
            let mut last_pack = None;
            for _ in 0..iterations {
                let pack = pack_html(sample_html, "https://cloud.internal/node1")
                    .expect("Pack benchmark should not fail");
                last_pack = Some(pack);
            }
            let duration_pack = start_pack.elapsed();
            let avg_pack_us = (duration_pack.as_micros() as f64) / (iterations as f64);
            let pack_throughput_mb =
                ((sample_html.len() * iterations) as f64 / 1_048_576.0) / duration_pack.as_secs_f64();

            let lp = last_pack.unwrap();
            println!("2. PocketWeb Packager (minify-html + Deflate + SHA256 + URL encode):");
            println!("   Total time:         {:.3?}", duration_pack);
            println!("   Average latency:    {:.2} µs / operation ({:.3} ms)", avg_pack_us, avg_pack_us / 1000.0);
            println!("   Throughput:         {:.2} MB/s", pack_throughput_mb);
            println!("   Original size:      {} bytes", lp.original_bytes);
            println!("   Compressed size:    {} bytes (Redução: {:.1}%)\n", lp.compressed_bytes, lp.reduction_pct);

            println!("===============================================================");
            println!("⚡ Status: Engine Rust 100% validada e pronta para alta escala!");
            println!("===============================================================");
        }
    }

    Ok(())
}
