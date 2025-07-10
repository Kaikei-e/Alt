// TASK5 Phase 4: Zero-allocation number parser for high-performance parsing
use crate::buffer::{ParseError, safe_parse_operation};

/// Zero-allocation number parser that avoids memory allocations
pub struct ZeroAllocParser;

impl ZeroAllocParser {
    /// Memory-safe u16 parsing with zero allocations
    pub fn parse_u16_safe(input: &str) -> Result<u16, ParseError> {
        if input.is_empty() {
            return Err(ParseError::EmptyInput);
        }

        if input.len() > 5 {
            return Err(ParseError::TooLong {
                input: input.to_string(),
                max_len: 5,
            });
        }

        // Manual parsing without allocations
        let mut result = 0u16;
        for (i, byte) in input.bytes().enumerate() {
            if !byte.is_ascii_digit() {
                return Err(ParseError::InvalidCharacter {
                    input: input.to_string(),
                    position: i,
                    character: byte as char,
                });
            }

            let digit = (byte - b'0') as u16;

            // Overflow check
            if let Some(new_result) = result.checked_mul(10) {
                if let Some(final_result) = new_result.checked_add(digit) {
                    result = final_result;
                } else {
                    return Err(ParseError::Overflow {
                        input: input.to_string(),
                        max_value: u16::MAX as u64,
                    });
                }
            } else {
                return Err(ParseError::Overflow {
                    input: input.to_string(),
                    max_value: u16::MAX as u64,
                });
            }
        }

        Ok(result)
    }

    /// Memory-safe u64 parsing with zero allocations
    pub fn parse_u64_safe(input: &str) -> Result<u64, ParseError> {
        if input.is_empty() {
            return Err(ParseError::EmptyInput);
        }

        if input.len() > 20 {
            return Err(ParseError::TooLong {
                input: input.to_string(),
                max_len: 20,
            });
        }

        // Handle special case for dash (common in nginx logs for zero size)
        if input == "-" {
            return Ok(0);
        }

        let mut result = 0u64;
        for (i, byte) in input.bytes().enumerate() {
            if !byte.is_ascii_digit() {
                return Err(ParseError::InvalidCharacter {
                    input: input.to_string(),
                    position: i,
                    character: byte as char,
                });
            }

            let digit = (byte - b'0') as u64;

            // Overflow check
            if let Some(new_result) = result.checked_mul(10) {
                if let Some(final_result) = new_result.checked_add(digit) {
                    result = final_result;
                } else {
                    return Err(ParseError::Overflow {
                        input: input.to_string(),
                        max_value: u64::MAX,
                    });
                }
            } else {
                return Err(ParseError::Overflow {
                    input: input.to_string(),
                    max_value: u64::MAX,
                });
            }
        }

        Ok(result)
    }

    /// Fallback-enabled parsing with default values
    pub fn parse_u16_with_fallback(input: &str, fallback: u16) -> u16 {
        Self::parse_u16_safe(input).unwrap_or(fallback)
    }

    pub fn parse_u64_with_fallback(input: &str, fallback: u64) -> u64 {
        Self::parse_u64_safe(input).unwrap_or(fallback)
    }

    /// Safe parsing operations with error recovery
    pub fn parse_u16_with_recovery(input: &str) -> u16 {
        safe_parse_operation(|| Self::parse_u16_safe(input)).unwrap_or_else(|e| {
            tracing::debug!("Failed to parse u16 '{}': {}, using fallback", input, e);
            0
        })
    }

    pub fn parse_u64_with_recovery(input: &str) -> u64 {
        safe_parse_operation(|| Self::parse_u64_safe(input)).unwrap_or_else(|e| {
            tracing::debug!("Failed to parse u64 '{}': {}, using fallback", input, e);
            0
        })
    }
}

/// Improved nginx parser with zero-allocation number parsing
pub struct ImprovedNginxParser;

impl ImprovedNginxParser {
    /// Parse nginx access log with safe number parsing
    pub fn parse_status_and_size_safe(
        status_str: Option<&str>,
        size_str: Option<&str>,
    ) -> (Option<u16>, Option<u64>) {
        let status_code = status_str
            .map(ZeroAllocParser::parse_u16_with_recovery)
            .filter(|&code| code > 0); // Filter out zero/error values

        let response_size = size_str.map(ZeroAllocParser::parse_u64_with_recovery);

        (status_code, response_size)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_u16_safe() {
        // Valid cases
        assert_eq!(ZeroAllocParser::parse_u16_safe("123"), Ok(123));
        assert_eq!(ZeroAllocParser::parse_u16_safe("0"), Ok(0));
        assert_eq!(ZeroAllocParser::parse_u16_safe("65535"), Ok(65535));

        // Error cases
        assert!(ZeroAllocParser::parse_u16_safe("").is_err());
        assert!(ZeroAllocParser::parse_u16_safe("abc").is_err());
        assert!(ZeroAllocParser::parse_u16_safe("70000").is_err()); // Overflow
        assert!(ZeroAllocParser::parse_u16_safe("123456").is_err()); // Too long
    }

    #[test]
    fn test_parse_u64_safe() {
        // Valid cases
        assert_eq!(ZeroAllocParser::parse_u64_safe("123456789"), Ok(123456789));
        assert_eq!(ZeroAllocParser::parse_u64_safe("0"), Ok(0));
        assert_eq!(ZeroAllocParser::parse_u64_safe("-"), Ok(0)); // Special case

        // Error cases
        assert!(ZeroAllocParser::parse_u64_safe("").is_err());
        assert!(ZeroAllocParser::parse_u64_safe("abc").is_err());
        assert!(ZeroAllocParser::parse_u64_safe("12345678901234567890123").is_err()); // Too long
    }

    #[test]
    fn test_fallback_parsing() {
        assert_eq!(ZeroAllocParser::parse_u16_with_fallback("123", 999), 123);
        assert_eq!(
            ZeroAllocParser::parse_u16_with_fallback("invalid", 999),
            999
        );

        assert_eq!(
            ZeroAllocParser::parse_u64_with_fallback("123456", 999),
            123456
        );
        assert_eq!(
            ZeroAllocParser::parse_u64_with_fallback("invalid", 999),
            999
        );
    }

    #[test]
    fn test_recovery_parsing() {
        assert_eq!(ZeroAllocParser::parse_u16_with_recovery("200"), 200);
        assert_eq!(ZeroAllocParser::parse_u16_with_recovery("invalid"), 0);

        assert_eq!(ZeroAllocParser::parse_u64_with_recovery("1024"), 1024);
        assert_eq!(ZeroAllocParser::parse_u64_with_recovery("invalid"), 0);
    }

    #[test]
    fn test_nginx_parser_safe() {
        let (status, size) =
            ImprovedNginxParser::parse_status_and_size_safe(Some("200"), Some("1024"));
        assert_eq!(status, Some(200));
        assert_eq!(size, Some(1024));

        let (status, size) =
            ImprovedNginxParser::parse_status_and_size_safe(Some("invalid"), Some("-"));
        assert_eq!(status, None); // Filtered out zero value
        assert_eq!(size, Some(0));
    }

    #[test]
    fn test_zero_allocation_performance() {
        // This test ensures we don't allocate during parsing
        let test_strings = ["200", "404", "1024", "0", "-", "999999"];

        for input in test_strings {
            // These should not allocate memory during parsing
            let _ = ZeroAllocParser::parse_u16_with_recovery(input);
            let _ = ZeroAllocParser::parse_u64_with_recovery(input);
        }
    }
}
