use std::io::Cursor;
use thumbnailer::error::ThumbResult;
use thumbnailer::{create_thumbnails, ThumbnailSize};

const PNG_BYTES: &'static [u8] = include_bytes!("../../inputs/magnetic-core.png");

enum SourceFormat {
    Png,
}

enum TargetFormat {
    Png,
}

fn main() {
    write_thumbnail(SourceFormat::Png, TargetFormat::Png).unwrap();
}

fn write_thumbnail(
    source_format: SourceFormat,
    target_format: TargetFormat,
) -> ThumbResult<Vec<u8>> {
    let thumb = match source_format {
        SourceFormat::Png => {
            let reader = Cursor::new(PNG_BYTES);
            create_thumbnails(reader, mime::IMAGE_PNG, [ThumbnailSize::Medium]).unwrap()
        }
    }
    .pop()
    .unwrap();

    let mut buf = Cursor::new(Vec::new());
    match target_format {
        TargetFormat::Png => thumb.write_png(&mut buf)?,
    }

    Ok(buf.into_inner())
}
