use std::path::{Path, PathBuf};

use async_zip::tokio::read::seek::ZipFileReader;
use tokio::{
    fs::{File, OpenOptions, create_dir_all},
    io::{BufReader, BufWriter},
};
use tokio_util::compat::FuturesAsyncReadCompatExt;

pub const URL: &str = "https://storage.googleapis.com/chrome-for-testing-public/117.0.5938.92/linux64/chrome-linux64.zip";

pub async fn fetch(to_path: &str) -> PathBuf {
    let zip_path = Path::new(to_path).join("chrome-linux64.zip");
    let out_dir = Path::new(to_path);
    let extracted_dir = out_dir.join("chrome-linux64");
    let bin_path = extracted_dir.join("chrome");
    if bin_path.exists() {
        println!("Browser already exists at {}", bin_path.display());
        return bin_path;
    }

    if !zip_path.exists() {
        let response = reqwest::get(URL).await.unwrap();
        let mut stream = response_to_async_read(response);
        let file = File::create(&zip_path).await.unwrap();
        let mut file = BufWriter::new(file);
        tokio::io::copy(&mut stream, &mut file).await.unwrap();
        println!("Browser downloaded to {}", zip_path.display());
    }

    unzip_file(File::open(&zip_path).await.unwrap(), &out_dir).await;
    println!("Browser extracted to {}", extracted_dir.display());

    assert!(
        bin_path.exists(),
        "Browser binary not found at {}",
        bin_path.display()
    );

    bin_path
}

fn response_to_async_read(resp: reqwest::Response) -> impl tokio::io::AsyncRead {
    use futures::stream::TryStreamExt;

    let stream = resp.bytes_stream().map_err(std::io::Error::other);
    tokio_util::io::StreamReader::new(stream)
}

async fn unzip_file(archive: File, out_dir: &Path) {
    let archive = BufReader::new(archive);
    let mut reader = ZipFileReader::with_tokio(archive)
        .await
        .expect("Failed to read zip file");
    for index in 0..reader.file().entries().len() {
        let entry = reader.file().entries().get(index).unwrap();
        let unix_perms = entry.unix_permissions().unwrap();
        let path = out_dir.join(sanitize_file_path(entry.filename().as_str().unwrap()));
        // If the filename of the entry ends with '/', it is treated as a directory.
        // This is implemented by previous versions of this crate and the Python Standard Library.
        // https://docs.rs/async_zip/0.0.8/src/async_zip/read/mod.rs.html#63-65
        // https://github.com/python/cpython/blob/820ef62833bd2d84a141adedd9a05998595d6b6d/Lib/zipfile.py#L528
        let entry_is_dir = entry.dir().unwrap();

        let entry_reader = reader
            .reader_without_entry(index)
            .await
            .expect("Failed to read ZipEntry");

        if entry_is_dir {
            // The directory may have been created if iteration is out of order.
            if !path.exists() {
                create_dir_all(&path)
                    .await
                    .expect("Failed to create extracted directory");
            }
        } else {
            // Creates parent directories. They may not exist if iteration is out of order
            // or the archive does not contain directory entries.
            let parent = path
                .parent()
                .expect("A file entry should have parent directories");
            if !parent.is_dir() {
                create_dir_all(parent)
                    .await
                    .expect("Failed to create parent directories");
            }
            {
                let writer = OpenOptions::new()
                    .write(true)
                    .create_new(true)
                    .open(&path)
                    .await
                    .expect("Failed to create extracted file");
                let mut writer = BufWriter::new(writer);

                let mut entry_reader = BufReader::new(entry_reader.compat());
                tokio::io::copy(&mut entry_reader, &mut writer)
                    .await
                    .expect("Failed to copy to extracted file");
            }
        }

        // Update file metadata based on entry metadata
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let permissions = std::fs::Permissions::from_mode(((unix_perms) << 16) as u32);
            std::fs::set_permissions(&path, permissions).expect("Failed to set file permissions");
        }
    }
}

fn sanitize_file_path(path: &str) -> PathBuf {
    // Replaces backwards slashes
    path.replace('\\', "/")
        // Sanitizes each component
        .split('/')
        .map(sanitize_filename::sanitize)
        .collect()
}
