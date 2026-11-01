import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers

// Reformat the approved artwork for the opaque 1024px App Store icon.
let input = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "web/public/diamond-way-logo-transparent.png"
let output = CommandLine.arguments.count > 2 ? CommandLine.arguments[2] : "ios/App/App/Assets.xcassets/AppIcon.appiconset/DWIcon.png"
guard let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: input) as CFURL, nil),
      let image = CGImageSourceCreateImageAtIndex(source, 0, nil),
      let context = CGContext(data: nil, width: 1024, height: 1024, bitsPerComponent: 8, bytesPerRow: 0, space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue) else {
    fatalError("Unable to load or render the application emblem")
}
context.setFillColor(CGColor(red: 0.965, green: 0.969, blue: 0.953, alpha: 1))
context.fill(CGRect(x: 0, y: 0, width: 1024, height: 1024))
let scale = min(800.0 / Double(image.width), 800.0 / Double(image.height))
let width = Double(image.width) * scale, height = Double(image.height) * scale
context.interpolationQuality = .high
context.draw(image, in: CGRect(x: (1024-width)/2, y: (1024-height)/2, width: width, height: height))
guard let result = context.makeImage(), let destination = CGImageDestinationCreateWithURL(URL(fileURLWithPath: output) as CFURL, UTType.png.identifier as CFString, 1, nil) else {
    fatalError("Unable to create the application icon")
}
CGImageDestinationAddImage(destination, result, nil)
precondition(CGImageDestinationFinalize(destination), "Unable to write the application icon")
