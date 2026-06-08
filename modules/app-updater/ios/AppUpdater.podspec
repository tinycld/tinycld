require 'json'
package = JSON.parse(File.read(File.join(__dir__, '..', 'package.json')))

Pod::Spec.new do |s|
  s.name           = 'AppUpdater'
  s.version        = package['version']
  s.summary        = package['description'] || 'Self-hosted OTA updater'
  s.platforms      = { :ios => '15.1' }
  s.source         = { git: '' }
  s.static_framework = true
  s.dependency 'ExpoModulesCore'
  s.dependency 'React-Core'
  s.source_files = '*.{swift,h,m}'
end
