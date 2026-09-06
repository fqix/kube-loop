Unicode true

####
## Windows installer for the Electron desktop application.
##
## The application ships its own Chromium runtime, so no WebView2 bootstrap is
## needed. This installs the packaged directory tree produced by Electron
## Forge, registers the kubeloop:// URL scheme, and stops the privileged helper
## service on uninstall.
##
## The packaging hook supplies every define:
##   makensis -DVERSION=3.0.0 -DARCH=amd64 \
##            -DSOURCE_DIR=<packaged app directory> \
##            -DOUT_FILE=<installer path> kubeloop.nsi
####

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef ARCH
  !define ARCH "amd64"
!endif
!ifndef SOURCE_DIR
  !error "SOURCE_DIR must point at the packaged application directory"
!endif
!ifndef OUT_FILE
  !error "OUT_FILE must name the installer to produce"
!endif

!define PRODUCT_NAME "KubeLoop"
!define COMPANY_NAME "KubeLoop"
!define PRODUCT_EXECUTABLE "KubeLoop.exe"
!define PROTOCOL_SCHEME "kubeloop"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${COMPANY_NAME}${PRODUCT_NAME}"

Name "${PRODUCT_NAME}"
OutFile "${OUT_FILE}"
InstallDir "$PROGRAMFILES64\${PRODUCT_NAME}"
RequestExecutionLevel admin
ShowInstDetails show
ManifestDPIAware true

VIProductVersion "${VERSION}.0"
VIFileVersion    "${VERSION}.0"
VIAddVersionKey "CompanyName"     "${COMPANY_NAME}"
VIAddVersionKey "FileDescription" "${PRODUCT_NAME} Installer"
VIAddVersionKey "ProductVersion"  "${VERSION}"
VIAddVersionKey "FileVersion"     "${VERSION}"
VIAddVersionKey "LegalCopyright"  "Copyright © ${COMPANY_NAME}"
VIAddVersionKey "ProductName"     "${PRODUCT_NAME}"

!include "MUI.nsh"

!define MUI_ICON "..\icons\appicon.ico"
!define MUI_UNICON "..\icons\appicon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Install"
    SetShellVarContext all

    # A running instance would hold the executable open.
    ExecWait 'taskkill /F /IM "${PRODUCT_EXECUTABLE}" /T' $0

    SetOutPath "$INSTDIR"
    File /r "${SOURCE_DIR}\*.*"

    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    # The OAuth login callback returns through this scheme.
    WriteRegStr HKCR "${PROTOCOL_SCHEME}" "" "URL:${PRODUCT_NAME} Protocol"
    WriteRegStr HKCR "${PROTOCOL_SCHEME}" "URL Protocol" ""
    WriteRegStr HKCR "${PROTOCOL_SCHEME}\DefaultIcon" "" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr HKCR "${PROTOCOL_SCHEME}\shell\open\command" "" '"$INSTDIR\${PRODUCT_EXECUTABLE}" "%1"'

    WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayName" "${PRODUCT_NAME}"
    WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
    WriteRegStr HKLM "${UNINSTALL_KEY}" "Publisher" "${COMPANY_NAME}"
    WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKLM "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKLM "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoModify" 1
    WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoRepair" 1

    WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section "uninstall"
    SetShellVarContext all

    ExecWait 'taskkill /F /IM "${PRODUCT_EXECUTABLE}" /T' $0

    # Stop and deregister the privileged helper service before deleting it.
    IfFileExists "$INSTDIR\resources\kubeloop-helper.exe" 0 +2
      ExecWait '"$INSTDIR\resources\kubeloop-helper.exe" uninstall'

    Delete "$SMPROGRAMS\${PRODUCT_NAME}.lnk"
    Delete "$DESKTOP\${PRODUCT_NAME}.lnk"

    DeleteRegKey HKCR "${PROTOCOL_SCHEME}"
    DeleteRegKey HKLM "${UNINSTALL_KEY}"

    RMDir /r "$INSTDIR"
SectionEnd
