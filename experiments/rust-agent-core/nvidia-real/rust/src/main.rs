//! A minimal real provider call to validate Rust compatibility with NVIDIA NIM.
//! No keys are stored. This is an isolated smoke test, not the full agent.
use reqwest::blocking::Client;
use reqwest::header::{ACCEPT, AUTHORIZATION};
use serde_json::{json, Value};
use std::env;
use std::error::Error;
use std::time::{Duration, Instant};

fn read_answer(body: &Value) -> Result<&str, Box<dyn Error>> {
    if let Some(error) = body.get("error") {
        return Err(format!("provider error: {}", error.get("type").and_then(Value::as_str).unwrap_or("unspecified")).into());
    }
    let answer = body.pointer("/choices/0/message/content")
        .and_then(Value::as_str)
        .filter(|s| !s.trim().is_empty())
        .ok_or("response missing nonempty choices[0].message.content")?;
    Ok(answer)
}

fn main() -> Result<(), Box<dyn Error>> {
    let api_key = env::var("NVIDIA_API_KEY").map_err(|_| "NVIDIA_API_KEY is required")?;
    if api_key.is_empty() { return Err("NVIDIA_API_KEY is empty".into()); }
    let model = env::var("NVIDIA_MODEL").unwrap_or_else(|_| "meta/llama-3.3-70b-instruct".to_owned());
    let url = env::var("NVIDIA_CHAT_URL")
        .unwrap_or_else(|_| "https://integrate.api.nvidia.com/v1/chat/completions".to_owned());
    let request = json!({
        "model": model,
        "messages": [
            {"role": "system", "content": "Responda de forma curta e objetiva."},
            {"role": "user", "content": "Diga apenas: teste de integracao"}
        ],
        "temperature": 0,
        "max_tokens": 32,
        "stream": false
    });
    let client = Client::builder().timeout(Duration::from_secs(90)).build()?;
    let start = Instant::now();
    let response = client.post(url)
        .header(AUTHORIZATION, format!("Bearer {api_key}"))
        .header(ACCEPT, "application/json")
        .json(&request).send()?;
    let code = response.status();
    if !code.is_success() {
        // Avoid logging response body or authorization headers.
        return Err(format!("NVIDIA HTTP status: {code}").into());
    }
    let body: Value = response.json()?;
    let answer = read_answer(&body)?;
    println!("language=rust status=ok duration_ms={} answer_present=true answer_chars={}",
        start.elapsed().as_millis(), answer.chars().count());
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn parses_answer() {
        let body = json!({"choices":[{"message":{"role":"assistant","content":"ok-local"}}]});
        assert_eq!(read_answer(&body).unwrap(), "ok-local");
    }
    #[test]
    fn rejects_empty_answer() {
        let body = json!({"choices":[{"message":{"content":""}}]});
        assert!(read_answer(&body).is_err());
    }
    #[test]
    fn rejects_provider_error() {
        let body = json!({"error":{"type":"rate_limit_error","message":"avoid logs"}});
        assert!(read_answer(&body).is_err());
    }
}
