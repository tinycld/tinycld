require 'json'
package = JSON.parse(File.read(File.join(__dir__, '..', 'package.json')))

Pod::Spec.new do |s|
  s.name           = 'EditorWebView'
  s.version        = package['version']
  s.summary        = package['description'] || 'Pooled editor WebView host'
  s.description    = package['description'] || 'Pooled editor WebView host'
  # Local, unpublished module — CocoaPods requires these attributes to validate,
  # so provide static values rather than reading from package.json.
  s.license        = { type: 'MIT' }
  s.author         = 'TinyCld'
  s.homepage       = 'https://tinycld.org'
  s.platforms      = { :ios => '15.1' }
  s.swift_version  = '5.9'
  s.source         = { git: 'https://github.com/tinycld/tinycld.git' }
  s.static_framework = true
  s.frameworks     = 'WebKit'
  s.dependency 'ExpoModulesCore'
  s.dependency 'React-Core'
  s.source_files = '*.{swift,h,m}'

  # DEFINES_MODULE puts the ObjC keyboard-hacks header in the module's umbrella
  # so the Swift pool can call it without an import.
  s.pod_target_xcconfig = { 'DEFINES_MODULE' => 'YES' }
end
