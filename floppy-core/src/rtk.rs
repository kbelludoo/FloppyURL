use regex::Regex;
use std::sync::OnceLock;

static RE_GIT_INDEX: OnceLock<Regex> = OnceLock::new();
static RE_MULTI_NEWLINES: OnceLock<Regex> = OnceLock::new();

/// Strips ANSI escape codes using a fast linear byte scanner without regex allocation overhead.
#[inline]
pub fn strip_ansi(input: &str) -> String {
    let bytes = input.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    let n = bytes.len();

    while i < n {
        if bytes[i] == 0x1b && i + 1 < n && bytes[i + 1] == b'[' {
            // ANSI escape sequence start: ESC [ ... [letter]
            i += 2;
            while i < n {
                let b = bytes[i];
                i += 1;
                if b.is_ascii_alphabetic() {
                    break;
                }
            }
        } else {
            out.push(bytes[i]);
            i += 1;
        }
    }

    String::from_utf8(out).unwrap_or_else(|_| input.to_string())
}

/// Compress command/tool outputs using RTK (Result Token Kompressor).
/// Removes ANSI sequences, git diff index metadata, trailing line spaces,
/// and collapses consecutive empty lines (3+ -> 2).
pub fn compress_tool_output(input: &str) -> String {
    if input.is_empty() {
        return String::new();
    }

    // 1. Strip ANSI escapes (ultra fast)
    let no_ansi = strip_ansi(input);

    // 2. Filter git index metadata and trim trailing line spaces
    let git_re = RE_GIT_INDEX.get_or_init(|| {
        Regex::new(r"(?i)^index [0-9a-f]{7,}\.\.[0-9a-f]{7,}( [0-9]+)?$").unwrap()
    });

    let mut filtered_lines = Vec::new();
    for line in no_ansi.lines() {
        let trimmed_end = line.trim_end_matches([' ', '\t', '\r']);
        if !git_re.is_match(trimmed_end) {
            filtered_lines.push(trimmed_end);
        }
    }

    let joined = filtered_lines.join("\n");

    // 3. Collapse 3+ newlines to 2
    let re_multi_newlines = RE_MULTI_NEWLINES.get_or_init(|| {
        Regex::new(r"\n{3,}").unwrap()
    });

    let collapsed = re_multi_newlines.replace_all(&joined, "\n\n");

    // 4. Trim bounding whitespace
    collapsed.trim().to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_strip_ansi() {
        let colored = "\x1b[32mSUCCESS\x1b[0m: Process \x1b[1;34m1234\x1b[0m completed.";
        assert_eq!(strip_ansi(colored), "SUCCESS: Process 1234 completed.");
    }

    #[test]
    fn test_compress_tool_output() {
        let raw = "\x1b[31merror\x1b[0m    \nindex a1b2c3d..e5f6a7b 100644\n\n\n\nline 1   \nline 2\t\t\n\n\nline 3\n\n";
        let compressed = compress_tool_output(raw);
        assert_eq!(compressed, "error\n\nline 1\nline 2\n\nline 3");
    }
}
