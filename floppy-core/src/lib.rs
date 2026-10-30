pub mod pack;
pub mod rtk;

pub use pack::{pack_html, PackResponse};
pub use rtk::{compress_tool_output, strip_ansi};

use std::ffi::{CStr, CString};
use std::os::raw::c_char;

/// C-ABI Interface: Compress Tool Output (RTK) for direct FFI linking with Go or C.
#[no_mangle]
pub extern "C" fn floppy_rtk_compress(input: *const c_char) -> *mut c_char {
    if input.is_null() {
        return std::ptr::null_mut();
    }
    let c_str = unsafe { CStr::from_ptr(input) };
    let input_str = match c_str.to_str() {
        Ok(s) => s,
        Err(_) => return std::ptr::null_mut(),
    };

    let result = compress_tool_output(input_str);
    match CString::new(result) {
        Ok(c_out) => c_out.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

/// Free string allocated by floppy_rtk_compress.
#[no_mangle]
pub extern "C" fn floppy_free_string(s: *mut c_char) {
    if !s.is_null() {
        unsafe {
            drop(CString::from_raw(s));
        }
    }
}
