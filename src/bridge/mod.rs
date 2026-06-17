use crate::mesh::MeshDevice;
use crate::ham::HamDevice;
use std::sync::Arc;

pub struct Bridge {
    mesh: Arc<dyn MeshDevice + Send + Sync>,
    ham: Arc<dyn HamDevice + Send + Sync>,
}





impl Bridge {
    

    pub fn new(
        mesh: Arc<dyn MeshDevice + Send + Sync>,
        ham: Arc<dyn HamDevice + Send + Sync>,
    ) -> Self {
        Self { mesh, ham }
    }

    pub async fn run(&self) {
        let mesh = Arc::clone(&self.mesh);
        let ham = Arc::clone(&self.ham);
        let mesh2 = Arc::clone(&self.mesh);
        let ham2 = Arc::clone(&self.ham);


        tokio::spawn(async move {
            loop {
                if let Ok(packet) = mesh.recv_raw_packet().await {
                    let _ = ham.send_frame(&packet).await;
                }
            }
        });

        tokio::spawn(async move {
            loop {
                if let Ok(frame) = ham2.recv_frame().await {
                    let _ = mesh2.send_packet(&frame).await;
                }
            }
        });
    }
}

