use aes_wasm::*;

fn loop_test_aes128ctr() {
    use aes128ctr::*;
    let key = b"fecoreencryption";
    let iv = IV::default();
    let msg = b"hello world";
    let mut count = 0u32;
    loop {
        count += 1;
        encrypt(msg, key, iv);
        if count == 10000 {
            //let output = String::from_utf8(plaintext);
            //println!("{} iterations on {:?}", count, output);
            break;
        }
    }   
}

fn main() {
    loop_test_aes128ctr();
}
