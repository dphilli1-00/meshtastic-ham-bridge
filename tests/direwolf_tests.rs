use meshtastic_ham_bridge::ham::HamDevice;
#[tokio::test]
async fn test_direwolf_kiss() {
    use tokio::net::TcpListener;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    // spawn fake direwolf
    let listener = TcpListener::bind("127.0.0.1:18001").await.unwrap();

    tokio::spawn(async move {
        let (mut socket, _) = listener.accept().await.unwrap();
        
        // read the KISS frame sent by DirewolfDevice
        let mut buf = vec![0u8; 64];
        let n = socket.read(&mut buf).await.unwrap();
        println!("fake direwolf received: {:?}", &buf[..n]);

        // send a KISS frame back: 0xC0 0x00 payload 0xC0
        let response = vec![0xC0u8, 0x00, b'h', b'i', 0xC0];
        socket.write_all(&response).await.unwrap();
    });

    // give listener time to start
    tokio::time::sleep(tokio::time::Duration::from_millis(10)).await;

    let device = meshtastic_ham_bridge::ham::direwolf::DirewolfDevice::connect("127.0.0.1", 18001)
        .await.unwrap();

    // test send
    device.send_frame(b"hello").await.unwrap();

    // test recv
    let frame = device.recv_frame().await.unwrap();
    println!("received frame: {:?}", frame);
    assert_eq!(frame, b"hi");
}