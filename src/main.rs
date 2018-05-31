extern crate byteorder;
extern crate tar;
extern crate tempfile;
extern crate tokio;
extern crate tokio_process;
extern crate uuid;

use std::io::{BufReader, BufWriter, Cursor};
use std::process::{Command, Stdio};

use byteorder::{BigEndian, ReadBytesExt};
use tempfile::tempdir;
use tokio::prelude::*;
use tokio::io;
use tokio::net::TcpListener;
use tokio_process::CommandExt;

fn main() {
    // Bind the server's socket.
    let addr = "0.0.0.0:9393".parse().unwrap();
    let listener = TcpListener::bind(&addr)
        .expect("unable to bind TCP listener");

    // Pull out a stream of sockets for incoming connections
    let server = listener.incoming()
        .map_err(|e| eprintln!("accept failed = {:?}", e))
        .for_each(|sock| {
            // Split up the reading and writing parts of the
            // socket.
            let (reader, writer) = sock.split();
            

            // get the size of the TAR file
            let size_buf = vec![0; 4];
            let handle_conn = io::read_exact(reader, size_buf)
                .and_then(|(reader, size_buf)| {
                    let size = Cursor::new(size_buf).read_u32::<BigEndian>().unwrap() as usize;
                    let w = vec![0u8; size];
                    io::copy(reader, BufWriter::new(Cursor::new(w)))
                })
                .and_then(|(_, _, buf)| {
                    let dir = tempdir().unwrap().into_path();
                    let tag = format!("registry.deployc/{}", uuid::Uuid::new_v4());
                    let cur = buf.into_inner().unwrap();
                    let tar_reader = BufReader::new(cur);
                    let mut archive = tar::Archive::new(tar_reader);
                    archive.unpack(&dir)?;
                    let mut build = Command::new("img")
                                            .arg("build")
                                            .arg("-t")
                                            .arg(&tag)
                                            .arg(dir)
                                            .stdout(Stdio::piped())
                                            .stderr(Stdio::piped())
                                            .spawn_async()?;
                    let stdout = build.stdout().take().unwrap();
                    let stderr = build.stderr().take().unwrap();
                    Ok((stdout, stderr, tag))
                })
                .and_then(|(stdout, stderr, tag)| {
                    io::copy(BufReader::new(stdout), writer)
                        .join(io::copy(BufReader::new(stderr), writer))
                        .and_then(|_| Ok(tag))
                })
                .and_then(|tag| {
                    let mut push = Command::new("img")
                                            .arg("push")
                                            .arg(&tag)
                                            .stdout(Stdio::piped())
                                            .stderr(Stdio::piped())
                                            .spawn_async()?;
                    let stdout = push.stdout().take().unwrap();
                    let stderr = push.stderr().take().unwrap();
                    Ok((stdout, stderr, tag))
                })
                .and_then(|(stdout, stderr, tag)| {
                    let w: BufWriter<Vec<u8>> = BufWriter::new(Vec::new());
                    io::copy(BufReader::new(stdout), writer)
                        .join(io::copy(BufReader::new(stderr), writer))
                        .and_then(|_| Ok(tag))
                })
                .then(|res| {
                    match res {
                        Ok(tag) => io::write_all(writer, tag.into_bytes()),
                        Err(err) => io::write_all(writer, format!("ERROR: {:?}", err).into_bytes())
                    }
                })
                .and_then(|_| Ok(()))
                .map_err(|err| eprintln!("Error: {:?}", err));

            // Spawn the future as a concurrent task.
            tokio::spawn(handle_conn)
        });

    // Start the Tokio runtime
    tokio::run(server);
}
