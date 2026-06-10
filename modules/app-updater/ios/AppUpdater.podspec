require 'json'
package = JSON.parse(File.read(File.join(__dir__, '..', 'package.json')))

Pod::Spec.new do |s|
  s.name           = 'AppUpdater'
  s.version        = package['version']
  s.summary        = package['description'] || 'Self-hosted OTA updater'
  s.description    = package['description'] || 'Self-hosted OTA updater'
  # Local, unpublished module — CocoaPods requires these attributes to validate,
  # so provide static values rather than reading from package.json.
  s.license        = { type: 'MIT' }
  s.author         = 'TinyCld'
  s.homepage       = 'https://tinycld.org'
  s.platforms      = { :ios => '15.1' }
  s.swift_version  = '5.9'
  s.source         = { git: 'https://github.com/tinycld/tinycld.git' }
  s.static_framework = true
  s.dependency 'ExpoModulesCore'
  s.dependency 'React-Core'
  s.source_files = '*.{swift,h,m}'

  # DEFINES_MODULE makes CocoaPods generate the Swift-interop header
  # (AppUpdater-Swift.h) and include it in the module's umbrella, so
  # `import AppUpdater` from AppDelegate.swift actually exposes the Swift
  # `@objc public class AppUpdaterBundle`. Without it the umbrella ships only
  # the ObjC headers (AppUpdaterReload.h) and the Swift class is invisible —
  # the build fails with "cannot find 'AppUpdaterBundle' in scope" in the
  # Release AppDelegate branch, even with the import present.
  s.pod_target_xcconfig = { 'DEFINES_MODULE' => 'YES' }
end
