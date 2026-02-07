use std::collections::HashMap;

/// Convert string to fixed-size byte array for FixedString columns
/// Pads with zeros if shorter, truncates if longer
#[must_use]
pub fn string_to_fixed_bytes<const N: usize>(s: &str) -> [u8; N] {
    let mut result = [0u8; N];
    let bytes = s.as_bytes();
    let len = bytes.len().min(N);
    result[..len].copy_from_slice(&bytes[..len]);
    result
}

/// Convert HashMap to Vec for ClickHouse Map type
#[must_use]
pub fn hashmap_to_vec<S: ::std::hash::BuildHasher>(
    map: HashMap<String, String, S>,
) -> Vec<(String, String)> {
    map.into_iter().collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    // =========================================================================
    // string_to_fixed_bytes tests
    // =========================================================================

    #[test]
    fn test_string_to_fixed_bytes_empty_string() {
        let result: [u8; 8] = string_to_fixed_bytes("");
        assert_eq!(result, [0u8; 8]);
    }

    #[test]
    fn test_string_to_fixed_bytes_shorter_than_n() {
        let result: [u8; 8] = string_to_fixed_bytes("abc");
        assert_eq!(result, [b'a', b'b', b'c', 0, 0, 0, 0, 0]);
    }

    #[test]
    fn test_string_to_fixed_bytes_exact_n_length() {
        let result: [u8; 4] = string_to_fixed_bytes("test");
        assert_eq!(result, [b't', b'e', b's', b't']);
    }

    #[test]
    fn test_string_to_fixed_bytes_longer_than_n_truncates() {
        let result: [u8; 4] = string_to_fixed_bytes("hello world");
        assert_eq!(result, [b'h', b'e', b'l', b'l']);
    }

    #[test]
    fn test_string_to_fixed_bytes_utf8_multibyte_boundary() {
        // "あ" is 3 bytes in UTF-8: [0xe3, 0x81, 0x82]
        let result: [u8; 4] = string_to_fixed_bytes("あ");
        assert_eq!(result, [0xe3, 0x81, 0x82, 0]);
    }

    #[test]
    fn test_string_to_fixed_bytes_trace_id_size() {
        let trace_id = "0123456789abcdef0123456789abcdef";
        let result: [u8; 32] = string_to_fixed_bytes(trace_id);
        assert_eq!(&result[..], trace_id.as_bytes());
    }

    #[test]
    fn test_string_to_fixed_bytes_span_id_size() {
        let span_id = "0123456789abcdef";
        let result: [u8; 16] = string_to_fixed_bytes(span_id);
        assert_eq!(&result[..], span_id.as_bytes());
    }

    #[test]
    fn test_string_to_fixed_bytes_zero_length_array() {
        let result: [u8; 0] = string_to_fixed_bytes("anything");
        assert_eq!(result.len(), 0);
    }

    // =========================================================================
    // hashmap_to_vec tests
    // =========================================================================

    #[test]
    fn test_hashmap_to_vec_empty() {
        let map: HashMap<String, String> = HashMap::new();
        let result = hashmap_to_vec(map);
        assert!(result.is_empty());
    }

    #[test]
    fn test_hashmap_to_vec_single_entry() {
        let mut map = HashMap::new();
        map.insert("key".to_string(), "value".to_string());
        let result = hashmap_to_vec(map);
        assert_eq!(result.len(), 1);
        assert!(result.contains(&("key".to_string(), "value".to_string())));
    }

    #[test]
    fn test_hashmap_to_vec_multiple_entries() {
        let mut map = HashMap::new();
        map.insert("a".to_string(), "1".to_string());
        map.insert("b".to_string(), "2".to_string());
        map.insert("c".to_string(), "3".to_string());
        let result = hashmap_to_vec(map);
        assert_eq!(result.len(), 3);
        assert!(result.contains(&("a".to_string(), "1".to_string())));
        assert!(result.contains(&("b".to_string(), "2".to_string())));
        assert!(result.contains(&("c".to_string(), "3".to_string())));
    }

    // =========================================================================
    // Property-based tests
    // =========================================================================

    mod prop {
        use super::*;
        use proptest::prelude::*;

        proptest! {
            #[test]
            fn string_to_fixed_bytes_output_length_always_n(s in ".*") {
                let result: [u8; 8] = string_to_fixed_bytes(&s);
                prop_assert_eq!(result.len(), 8);

                let result: [u8; 32] = string_to_fixed_bytes(&s);
                prop_assert_eq!(result.len(), 32);
            }

            #[test]
            fn string_to_fixed_bytes_prefix_matches_input(s in "[a-zA-Z0-9]{0,16}") {
                let result: [u8; 32] = string_to_fixed_bytes(&s);
                let input_bytes = s.as_bytes();
                let check_len = input_bytes.len().min(32);
                prop_assert_eq!(&result[..check_len], &input_bytes[..check_len]);
            }

            #[test]
            fn string_to_fixed_bytes_pads_with_zeros(s in "[a-z]{0,4}") {
                let result: [u8; 8] = string_to_fixed_bytes(&s);
                let input_len = s.len().min(8);
                for &b in &result[input_len..] {
                    prop_assert_eq!(b, 0, "trailing bytes must be zero-padded");
                }
            }

            #[test]
            fn hashmap_to_vec_preserves_all_entries(
                entries in proptest::collection::vec(("[a-z]{1,8}", "[a-z]{1,8}"), 0..20)
            ) {
                let mut map = HashMap::new();
                for (k, v) in &entries {
                    map.insert(k.clone(), v.clone());
                }
                let expected_len = map.len();
                let result = hashmap_to_vec(map);
                prop_assert_eq!(result.len(), expected_len);
            }
        }
    }
}
