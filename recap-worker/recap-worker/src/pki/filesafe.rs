//! Directory-fd + `O_NOFOLLOW` I/O so secret and cert paths cannot be swapped
//! through a symlink between `stat` and `open` (F-002).

use std::ffi::CString;
use std::fs::File;
use std::io::{Read, Write};
use std::os::fd::FromRawFd;
use std::os::unix::ffi::OsStrExt;
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::os::unix::io::AsRawFd;
use std::path::{Path, PathBuf};

use super::error::PkiError;

pub(crate) const MAX_PASSWORD_BYTES: u64 = 4 << 10;
pub(crate) const MAX_ENV_FILE_BYTES: u64 = 4 << 10;
pub(crate) const MAX_ROOT_PEM_BYTES: u64 = 1 << 20;

pub(crate) fn assert_trusted_parent(path: &Path) -> Result<(), PkiError> {
    if path.as_os_str().is_empty() {
        return Err(PkiError::other("empty path"));
    }
    let dir = path
        .parent()
        .filter(|p| !p.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    let info = std::fs::symlink_metadata(dir)
        .map_err(|e| PkiError::other(format!("lstat parent of {}: {e}", path.display())))?;
    if info.file_type().is_symlink() {
        return Err(PkiError::Symlink {
            path: dir.display().to_string(),
        });
    }
    if !info.is_dir() {
        return Err(PkiError::other(format!(
            "parent of {} is not a directory",
            path.display()
        )));
    }
    Ok(())
}

pub(crate) fn assert_trusted_dest(path: &Path) -> Result<(), PkiError> {
    assert_trusted_parent(path)?;
    match std::fs::symlink_metadata(path) {
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(err) => Err(PkiError::other(format!("lstat {}: {err}", path.display()))),
        Ok(info) if info.file_type().is_symlink() => Err(PkiError::Symlink {
            path: path.display().to_string(),
        }),
        Ok(_) => Ok(()),
    }
}

pub(crate) fn read_regular_no_follow(path: &Path, max_bytes: u64) -> Result<Vec<u8>, PkiError> {
    assert_trusted_parent(path)?;
    let dir = path
        .parent()
        .filter(|p| !p.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    let base = path
        .file_name()
        .and_then(|n| n.to_str())
        .ok_or_else(|| PkiError::other(format!("invalid basename for {}", path.display())))?;
    if base == "." || base == ".." || base.contains('/') {
        return Err(PkiError::other(format!(
            "invalid basename for {}",
            path.display()
        )));
    }
    let dir_c = CString::new(dir.as_os_str().as_bytes())
        .map_err(|_| PkiError::other(format!("invalid parent path for {}", path.display())))?;
    let dir_fd = unsafe {
        libc::open(
            dir_c.as_ptr(),
            libc::O_RDONLY | libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC,
        )
    };
    if dir_fd < 0 {
        let err = std::io::Error::last_os_error();
        if err.raw_os_error() == Some(libc::ELOOP) {
            return Err(PkiError::Symlink {
                path: dir.display().to_string(),
            });
        }
        return Err(PkiError::other(format!(
            "open parent of {}: {err}",
            path.display()
        )));
    }
    let dirf = unsafe { File::from_raw_fd(dir_fd) };
    let c_name = CString::new(base)
        .map_err(|_| PkiError::other(format!("invalid basename for {}", path.display())))?;
    let fd = unsafe {
        libc::openat(
            dirf.as_raw_fd(),
            c_name.as_ptr(),
            libc::O_RDONLY | libc::O_NOFOLLOW | libc::O_CLOEXEC,
        )
    };
    if fd < 0 {
        let err = std::io::Error::last_os_error();
        if err.raw_os_error() == Some(libc::ELOOP) {
            return Err(PkiError::Symlink {
                path: path.display().to_string(),
            });
        }
        return Err(PkiError::other(format!("open {}: {err}", path.display())));
    }
    let file = unsafe { File::from_raw_fd(fd) };
    let meta = file
        .metadata()
        .map_err(|e| PkiError::other(format!("fstat {}: {e}", path.display())))?;
    if !meta.is_file() {
        return Err(PkiError::other(format!(
            "{} is not a regular file",
            path.display()
        )));
    }
    if meta.permissions().mode() & 0o002 != 0 {
        return Err(PkiError::other(format!(
            "{} is world-writable",
            path.display()
        )));
    }
    if meta.len() > max_bytes {
        return oversized(path, max_bytes);
    }
    let mut raw = Vec::new();
    file.take(max_bytes.saturating_add(1))
        .read_to_end(&mut raw)
        .map_err(|e| PkiError::other(format!("read {}: {e}", path.display())))?;
    if u64::try_from(raw.len()).unwrap_or(u64::MAX) > max_bytes {
        return oversized(path, max_bytes);
    }
    Ok(raw)
}

fn oversized(path: &Path, max_bytes: u64) -> Result<Vec<u8>, PkiError> {
    if max_bytes == MAX_PASSWORD_BYTES {
        Err(PkiError::PasswordTooLarge)
    } else {
        Err(PkiError::other(format!(
            "{} exceeds {max_bytes}-byte cap",
            path.display()
        )))
    }
}

pub(crate) fn write_temp_nofollow(
    final_path: &Path,
    data: &[u8],
    mode: u32,
) -> Result<PathBuf, PkiError> {
    assert_trusted_dest(final_path)?;
    let dir = final_path
        .parent()
        .filter(|p| !p.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    std::fs::create_dir_all(dir).map_err(|e| PkiError::other(format!("mkdir temp: {e}")))?;
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_or(0, |d| d.as_nanos());
    let tmp_name = dir.join(format!(".pki-enroll-{nanos}-{}", std::process::id()));
    let mut tmp = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(mode)
        .custom_flags(libc::O_NOFOLLOW)
        .open(&tmp_name)
        .map_err(|e| PkiError::other(format!("create temp: {e}")))?;
    if let Err(err) = tmp.write_all(data) {
        let _ = std::fs::remove_file(&tmp_name);
        return Err(PkiError::other(format!("write temp: {err}")));
    }
    if let Err(err) = tmp.sync_all() {
        let _ = std::fs::remove_file(&tmp_name);
        return Err(PkiError::other(format!("sync temp: {err}")));
    }
    drop(tmp);
    let _ = std::fs::set_permissions(&tmp_name, std::fs::Permissions::from_mode(mode));
    Ok(tmp_name)
}

fn parent_dir(path: &Path) -> &Path {
    path.parent()
        .filter(|p| !p.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."))
}

fn file_basename(path: &Path) -> Result<&str, PkiError> {
    let base = path
        .file_name()
        .and_then(|n| n.to_str())
        .ok_or_else(|| PkiError::other(format!("invalid basename for {}", path.display())))?;
    if base == "." || base == ".." || base.contains('/') {
        return Err(PkiError::other(format!(
            "invalid basename for {}",
            path.display()
        )));
    }
    Ok(base)
}

pub(crate) fn open_parent_dirfd(path: &Path) -> Result<File, PkiError> {
    assert_trusted_parent(path)?;
    let dir = parent_dir(path);
    let dir_c = CString::new(dir.as_os_str().as_bytes())
        .map_err(|_| PkiError::other(format!("invalid parent path for {}", path.display())))?;
    let dir_fd = unsafe {
        libc::open(
            dir_c.as_ptr(),
            libc::O_RDONLY | libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC,
        )
    };
    if dir_fd < 0 {
        let err = std::io::Error::last_os_error();
        if err.raw_os_error() == Some(libc::ELOOP) {
            return Err(PkiError::Symlink {
                path: dir.display().to_string(),
            });
        }
        return Err(PkiError::other(format!(
            "open parent of {}: {err}",
            path.display()
        )));
    }
    Ok(unsafe { File::from_raw_fd(dir_fd) })
}

pub(crate) fn rename_nofollow(src: &Path, dst: &Path) -> Result<(), PkiError> {
    if parent_dir(src).as_os_str() != parent_dir(dst).as_os_str() {
        return Err(PkiError::other(format!(
            "rename {} -> {} crosses directories",
            src.display(),
            dst.display()
        )));
    }
    assert_trusted_dest(dst)?;
    let dirf = open_parent_dirfd(src)?;
    let src_c = CString::new(file_basename(src)?)
        .map_err(|_| PkiError::other(format!("invalid basename for {}", src.display())))?;
    let dst_c = CString::new(file_basename(dst)?)
        .map_err(|_| PkiError::other(format!("invalid basename for {}", dst.display())))?;
    let rc = unsafe {
        libc::renameat(
            dirf.as_raw_fd(),
            src_c.as_ptr(),
            dirf.as_raw_fd(),
            dst_c.as_ptr(),
        )
    };
    if rc != 0 {
        return Err(PkiError::other(format!(
            "rename {} -> {}: {}",
            src.display(),
            dst.display(),
            std::io::Error::last_os_error()
        )));
    }
    Ok(())
}

pub(crate) fn unlink_nofollow(path: &Path) -> Result<(), PkiError> {
    let dirf = match open_parent_dirfd(path) {
        Ok(file) => file,
        Err(err) => {
            if std::fs::symlink_metadata(parent_dir(path))
                .err()
                .is_some_and(|e| e.kind() == std::io::ErrorKind::NotFound)
            {
                return Ok(());
            }
            return Err(err);
        }
    };
    let name = CString::new(file_basename(path)?)
        .map_err(|_| PkiError::other(format!("invalid basename for {}", path.display())))?;
    let rc = unsafe { libc::unlinkat(dirf.as_raw_fd(), name.as_ptr(), 0) };
    if rc != 0 {
        let err = std::io::Error::last_os_error();
        if err.kind() == std::io::ErrorKind::NotFound || err.raw_os_error() == Some(libc::ENOENT) {
            return Ok(());
        }
        return Err(PkiError::other(format!("unlink {}: {err}", path.display())));
    }
    Ok(())
}

pub(crate) fn write_regular_nofollow(path: &Path, data: &[u8], mode: u32) -> Result<(), PkiError> {
    let tmp = write_temp_nofollow(path, data, mode)?;
    if let Err(err) = rename_nofollow(&tmp, path) {
        let _ = unlink_nofollow(&tmp);
        return Err(err);
    }
    Ok(())
}
