import AppKit
import Foundation
import WebKit

struct Options {
    let inputURL: URL
    let outputURL: URL
    let width: Double
    let waitMS: Int
    let hideSelectors: [String]
}

enum CLIError: Error {
    case message(String)
}

func parseArgs() throws -> Options {
    var input: String?
    var output: String?
    var width = 1440.0
    var waitMS = 1000
    var hideSelectors: [String] = []
    var i = 1
    while i < CommandLine.arguments.count {
        let arg = CommandLine.arguments[i]
        switch arg {
        case "--input":
            i += 1
            guard i < CommandLine.arguments.count else {
                throw CLIError.message("Missing value for --input")
            }
            input = CommandLine.arguments[i]
        case "--output":
            i += 1
            guard i < CommandLine.arguments.count else {
                throw CLIError.message("Missing value for --output")
            }
            output = CommandLine.arguments[i]
        case "--width":
            i += 1
            guard i < CommandLine.arguments.count, let value = Double(CommandLine.arguments[i]) else {
                throw CLIError.message("Invalid value for --width")
            }
            width = value
        case "--wait-ms":
            i += 1
            guard i < CommandLine.arguments.count, let value = Int(CommandLine.arguments[i]) else {
                throw CLIError.message("Invalid value for --wait-ms")
            }
            waitMS = value
        case "--hide-selector":
            i += 1
            guard i < CommandLine.arguments.count else {
                throw CLIError.message("Missing value for --hide-selector")
            }
            hideSelectors.append(CommandLine.arguments[i])
        default:
            throw CLIError.message("Unknown argument: \(arg)")
        }
        i += 1
    }
    guard let input, let output else {
        throw CLIError.message("Usage: swift render_webkit_pdf.swift --input <url> --output <pdf> [--width 1440] [--wait-ms 1000] [--hide-selector <css>]")
    }
    guard let inputURL = URL(string: input) else {
        throw CLIError.message("Invalid input URL: \(input)")
    }
    return Options(
        inputURL: inputURL,
        outputURL: URL(fileURLWithPath: output),
        width: width,
        waitMS: waitMS,
        hideSelectors: hideSelectors
    )
}

final class Exporter: NSObject, WKNavigationDelegate {
    private let options: Options
    private let webView: WKWebView

    init(options: Options) {
        self.options = options
        self.webView = WKWebView(frame: NSRect(x: 0, y: 0, width: options.width, height: 1800))
        super.init()
        self.webView.navigationDelegate = self
    }

    func run() {
        let request = URLRequest(
            url: options.inputURL,
            cachePolicy: .reloadIgnoringLocalCacheData,
            timeoutInterval: 60
        )
        webView.load(request)
        RunLoop.current.run()
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        fputs("WebKit provisional navigation failed: \(error)\n", stderr)
        exit(1)
    }

    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        fputs("WebKit navigation failed: \(error)\n", stderr)
        exit(1)
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        let delay = DispatchTimeInterval.milliseconds(options.waitMS)
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) { [self] in
            measureAndExport()
        }
    }

    private func measureAndExport() {
        guard let hideJSON = makeJSONString(options.hideSelectors) else {
            fputs("Failed to encode hide selectors\n", stderr)
            exit(1)
        }
        let script = """
        (() => {
          const selectors = \(hideJSON);
          for (const selector of selectors) {
            for (const node of document.querySelectorAll(selector)) {
              node.style.display = 'none';
            }
          }
          const root = document.documentElement;
          const body = document.body;
          const candidates = [
            root ? root.scrollHeight : 0,
            root ? root.offsetHeight : 0,
            root ? root.clientHeight : 0,
            body ? body.scrollHeight : 0,
            body ? body.offsetHeight : 0,
            body ? body.clientHeight : 0,
            body ? Math.ceil(body.getBoundingClientRect().bottom + window.scrollY) : 0,
            window.innerHeight
          ];
          for (const node of document.querySelectorAll('main, [role="main"]')) {
            const rect = node.getBoundingClientRect();
            candidates.push(Math.ceil(rect.bottom + window.scrollY + 24));
          }
          return JSON.stringify({ height: Math.max(...candidates) });
        })();
        """
        webView.evaluateJavaScript(script) { [self] result, error in
            if let error {
                fputs("evaluateJavaScript failed: \(error)\n", stderr)
                exit(1)
            }
            guard let raw = result as? String,
                  let data = raw.data(using: .utf8),
                  let payload = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let height = payload["height"] as? Double else {
                fputs("Invalid height payload from page\n", stderr)
                exit(1)
            }
            exportPDF(height: max(height, 1))
        }
    }

    private func exportPDF(height: Double) {
        webView.setFrameSize(NSSize(width: options.width, height: height))
        let config = WKPDFConfiguration()
        config.rect = CGRect(x: 0, y: 0, width: options.width, height: height)
        webView.createPDF(configuration: config) { [self] result in
            do {
                let data = try result.get()
                try data.write(to: options.outputURL)
                print(options.outputURL.path)
                exit(0)
            } catch {
                fputs("PDF export failed: \(error)\n", stderr)
                exit(1)
            }
        }
    }

    private func makeJSONString(_ selectors: [String]) -> String? {
        guard let data = try? JSONSerialization.data(withJSONObject: selectors, options: []),
              let text = String(data: data, encoding: .utf8) else {
            return nil
        }
        return text
    }
}

do {
    _ = NSApplication.shared
    let options = try parseArgs()
    Exporter(options: options).run()
} catch let error as CLIError {
    switch error {
    case .message(let message):
        fputs("\(message)\n", stderr)
        exit(2)
    }
} catch {
    fputs("Unexpected error: \(error)\n", stderr)
    exit(1)
}
