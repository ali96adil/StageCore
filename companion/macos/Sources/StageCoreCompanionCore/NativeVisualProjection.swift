#if os(macOS)
import Foundation
import QuartzCore

func makeNativeVisualPerspectiveTransform(
    width: Double,
    height: Double,
    mapping: VisualQuadState
) throws -> CATransform3D {
    guard width.isFinite, height.isFinite, width > 0, height > 0,
          mapping.isValidProjection else {
        throw VisualRendererFailure(
            code: "VISUAL_RENDERER_MAPPING_INVALID",
            summary: "projector mapping is invalid or degenerate"
        )
    }

    let x0 = mapping.topLeft.x * width
    let y0 = mapping.topLeft.y * height
    let x1 = mapping.topRight.x * width
    let y1 = mapping.topRight.y * height
    let x2 = mapping.bottomRight.x * width
    let y2 = mapping.bottomRight.y * height
    let x3 = mapping.bottomLeft.x * width
    let y3 = mapping.bottomLeft.y * height

    let dx3 = x0 - x1 + x2 - x3
    let dy3 = y0 - y1 + y2 - y3

    let a: Double
    let b: Double
    let c = x0
    let d: Double
    let e: Double
    let f = y0
    let g: Double
    let h: Double

    if abs(dx3) < 1e-12 && abs(dy3) < 1e-12 {
        a = x1 - x0
        b = x3 - x0
        d = y1 - y0
        e = y3 - y0
        g = 0
        h = 0
    } else {
        let dx1 = x1 - x2
        let dx2 = x3 - x2
        let dy1 = y1 - y2
        let dy2 = y3 - y2
        let denominator = dx1 * dy2 - dx2 * dy1
        guard denominator.isFinite, abs(denominator) > 1e-12 else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_MAPPING_INVALID",
                summary: "projector mapping cannot produce a stable perspective transform"
            )
        }
        g = (dx3 * dy2 - dx2 * dy3) / denominator
        h = (dx1 * dy3 - dx3 * dy1) / denominator
        a = x1 - x0 + g * x1
        b = x3 - x0 + h * x3
        d = y1 - y0 + g * y1
        e = y3 - y0 + h * y3
    }

    var transform = CATransform3DIdentity
    transform.m11 = a / width
    transform.m21 = b / height
    transform.m41 = c
    transform.m12 = d / width
    transform.m22 = e / height
    transform.m42 = f
    transform.m14 = g / width
    transform.m24 = h / height
    transform.m44 = 1

    let values = [
        transform.m11, transform.m12, transform.m14,
        transform.m21, transform.m22, transform.m24,
        transform.m41, transform.m42, transform.m44,
    ]
    guard values.allSatisfy({ $0.isFinite }) else {
        throw VisualRendererFailure(
            code: "VISUAL_RENDERER_MAPPING_INVALID",
            summary: "projector mapping produced non-finite transform coefficients"
        )
    }
    return transform
}
#endif
