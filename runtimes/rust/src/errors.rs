use std::fmt;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ErrorCode {
    Ok = 0,
    InvalidArgument = 3,
    NotFound = 5,
    AlreadyExists = 6,
    PermissionDenied = 7,
    Unimplemented = 12,
    Internal = 13,
    Unavailable = 14,
    Unauthenticated = 16,
}

#[derive(Debug, Clone)]
pub struct HelixError {
    pub code: ErrorCode,
    pub message: String,
}

impl fmt::Display for HelixError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "helix error: code={:?} message={}",
            self.code, self.message
        )
    }
}

impl std::error::Error for HelixError {}

impl HelixError {
    pub fn new(code: ErrorCode, message: &str) -> Self {
        HelixError {
            code,
            message: message.to_string(),
        }
    }

    pub fn to_thrift_error(&self) -> thrift::Error {
        match self.code {
            ErrorCode::Unimplemented => thrift::Error::Application(thrift::ApplicationError::new(
                thrift::ApplicationErrorKind::UnknownMethod,
                self.message.clone(),
            )),
            ErrorCode::InvalidArgument => {
                thrift::Error::Application(thrift::ApplicationError::new(
                    thrift::ApplicationErrorKind::ProtocolError,
                    self.message.clone(),
                ))
            }
            _ => thrift::Error::Application(thrift::ApplicationError::new(
                thrift::ApplicationErrorKind::InternalError,
                self.message.clone(),
            )),
        }
    }
}
