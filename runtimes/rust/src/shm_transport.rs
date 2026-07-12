#[cfg(unix)]
use std::fs::OpenOptions;
#[cfg(unix)]
use std::os::unix::io::AsRawFd;
use std::sync::Arc;
#[cfg(unix)]
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;
use tokio::sync::Mutex;

pub struct ShmConn {
    pub signal: TcpStream,
    #[cfg(unix)]
    mmap_ptr: *mut libc::c_void,
    size: usize,
    write_mu: Arc<Mutex<()>>,
}

// Safely implement Send and Sync since access to mmap pointer is guarded/isolated
unsafe impl Send for ShmConn {}
unsafe impl Sync for ShmConn {}

impl ShmConn {
    #[cfg(unix)]
    pub fn new(
        signal: TcpStream,
        shm_path: &str,
        size: usize,
        is_owner: bool,
    ) -> Result<Self, std::io::Error> {
        let file = if is_owner {
            let f = OpenOptions::new()
                .read(true)
                .write(true)
                .create(true)
                .truncate(true)
                .open(shm_path)?;
            f.set_len(size as u64)?;
            f
        } else {
            OpenOptions::new().read(true).write(true).open(shm_path)?
        };

        let mmap_ptr = unsafe {
            libc::mmap(
                std::ptr::null_mut(),
                size,
                libc::PROT_READ | libc::PROT_WRITE,
                libc::MAP_SHARED,
                file.as_raw_fd(),
                0,
            )
        };

        if mmap_ptr == libc::MAP_FAILED {
            return Err(std::io::Error::last_os_error());
        }

        Ok(ShmConn {
            signal,
            mmap_ptr,
            size,
            write_mu: Arc::new(Mutex::new(())),
        })
    }

    #[cfg(not(unix))]
    pub fn new(
        _signal: TcpStream,
        _shm_path: &str,
        _size: usize,
        _is_owner: bool,
    ) -> Result<Self, std::io::Error> {
        Err(std::io::Error::new(
            std::io::ErrorKind::Unsupported,
            "Shared memory transport is not supported on Windows",
        ))
    }

    #[cfg(unix)]
    pub async fn read_payload(&mut self) -> Result<Vec<u8>, std::io::Error> {
        let mut header = [0u8; 8];
        self.signal.read_exact(&mut header).await?;

        let offset = u32::from_be_bytes([header[0], header[1], header[2], header[3]]) as usize;
        let length = u32::from_be_bytes([header[4], header[5], header[6], header[7]]) as usize;

        if offset + length > self.size {
            return Err(std::io::Error::new(
                std::io::ErrorKind::InvalidData,
                "shm read out of bounds",
            ));
        }

        let mut data = vec![0u8; length];
        unsafe {
            let src = (self.mmap_ptr as *const u8).add(offset);
            std::ptr::copy_nonoverlapping(src, data.as_mut_ptr(), length);
        }

        Ok(data)
    }

    #[cfg(not(unix))]
    pub async fn read_payload(&mut self) -> Result<Vec<u8>, std::io::Error> {
        Err(std::io::Error::new(
            std::io::ErrorKind::Unsupported,
            "Shared memory transport is not supported on Windows",
        ))
    }

    #[cfg(unix)]
    pub async fn write_payload(&mut self, payload: &[u8]) -> Result<(), std::io::Error> {
        let _guard = self.write_mu.lock().await;

        let length = payload.len();
        if length > self.size {
            return Err(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "payload size exceeds shm capacity",
            ));
        }

        let offset = 0; // ping-pong model

        unsafe {
            let dest = (self.mmap_ptr as *mut u8).add(offset);
            std::ptr::copy_nonoverlapping(payload.as_ptr(), dest, length);
        }

        let mut header = [0u8; 8];
        header[0..4].copy_from_slice(&(offset as u32).to_be_bytes());
        header[4..8].copy_from_slice(&(length as u32).to_be_bytes());

        self.signal.write_all(&header).await?;
        self.signal.flush().await?;

        Ok(())
    }

    #[cfg(not(unix))]
    pub async fn write_payload(&mut self, _payload: &[u8]) -> Result<(), std::io::Error> {
        Err(std::io::Error::new(
            std::io::ErrorKind::Unsupported,
            "Shared memory transport is not supported on Windows",
        ))
    }
}

impl Drop for ShmConn {
    fn drop(&mut self) {
        #[cfg(unix)]
        unsafe {
            libc::munmap(self.mmap_ptr, self.size);
        }
    }
}
