// Port of the orchestration state machine from pkg/agent/agent.go.
// A deterministic fake LLM + tool are used to benchmark local overhead only.
// This is NOT a complete port of FloppyURL.
use std::env;

#[derive(Clone)]
struct ToolCall { id: String, name: String, arguments: String }
#[derive(Clone)]
struct Message {
    role: &'static str,
    content: String,
    tool_call_id: String,
    tool_calls: Vec<ToolCall>,
}
impl Message {
    fn new(role: &'static str, content: String) -> Self {
        Self { role, content, tool_call_id: String::new(), tool_calls: Vec::new() }
    }
}
struct Agent { history: Vec<Message>, max_rounds: usize }
impl Agent {
    fn new() -> Self { Self { history: Vec::new(), max_rounds: 20 } }
    fn run(&mut self, input: &str) -> Result<String, String> {
        let mut messages = Vec::with_capacity(self.history.len()+6);
        messages.push(Message::new("system", "You are an AI agent.".into()));
        messages.extend(self.history.iter().cloned());
        messages.push(Message::new("user", input.to_owned()));
        for _ in 0..self.max_rounds {
            let (reply, stop) = complete(&messages);
            let no_tools = reply.tool_calls.is_empty();
            messages.push(reply.clone());
            if stop || no_tools {
                self.history.push(Message::new("user", input.to_owned()));
                self.history.push(reply.clone());
                if self.history.len() > 40 {
                    self.history.drain(..self.history.len()-40);
                }
                let result = reply.content.trim();
                return Ok(if result.is_empty() {"(sem resposta)".into()} else {result.to_owned()});
            }
            for call in &reply.tool_calls {
                let mut msg = Message::new("tool", execute_tool(call));
                msg.tool_call_id = call.id.clone();
                messages.push(msg);
            }
        }
        Err(format!("agent reached maximum rounds ({})", self.max_rounds))
    }
}
fn complete(messages: &[Message]) -> (Message, bool) {
    let last = messages.last().expect("at least a user message");
    if last.role == "tool" {
        return (Message::new("assistant", format!("feito:{}",last.content)), true);
    }
    let mut reply = Message::new("assistant", String::new());
    reply.tool_calls.push(ToolCall {
        id: "call_0".into(), name: "echo".into(), arguments: last.content.clone(),
    });
    (reply,false)
}
fn execute_tool(call: &ToolCall) -> String {
    if call.name == "echo" {format!("ok:{}",call.arguments)}
    else {"Tool error: unknown tool".into()}
}
fn main() {
    let n = env::args().nth(1).map(|s|s.parse::<usize>().expect("invalid iterations")).unwrap_or(250000);
    assert!(n>0);
    let mut agent = Agent::new();
    let mut checksum: u64 = 0;
    for i in 0..n {
        let input = format!("pedido-{}",i%100);
        let result = agent.run(&input).expect("agent failed");
        checksum += result.len() as u64;
    }
    println!("language=rust iterations={} checksum={} history={}",n,checksum,agent.history.len());
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn round_trip() { assert_eq!(Agent::new().run("pedido-7").unwrap(),"feito:ok:pedido-7"); }
    #[test]
    fn history_limit() {
        let mut a=Agent::new();
        for _ in 0..40 { a.run("oi").unwrap(); }
        assert_eq!(a.history.len(),40);
    }
    #[test]
    fn round_limit() {
        let mut a=Agent::new();
        a.max_rounds=1;
        assert!(a.run("oi").is_err());
    }
}
