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
end
