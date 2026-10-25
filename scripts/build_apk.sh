#!/usr/bin/env bash
set -e

SDK_HOME="/opt/android-sdk"
BUILD_TOOLS="${SDK_HOME}/build-tools/33.0.2"
PLATFORM_JAR="${SDK_HOME}/platforms/android-33/android.jar"

AAR_PATH="dist/gosms-android/gosms.aar"
FINAL_APK="dist/gosms-android/gosms.apk"
WORK_DIR="/tmp/gosms-apk-builder"

rm -rf "${WORK_DIR}"
mkdir -p "${WORK_DIR}/java/com/egonzalez/gosms"
mkdir -p "${WORK_DIR}/dex"
mkdir -p "${WORK_DIR}/compiled_res"

# 1. Extraer classes.jar y bibliotecas nativas (.so) del .aar
unzip -q -o "${AAR_PATH}" -d "${WORK_DIR}/aar_contents"
cp "${WORK_DIR}/aar_contents/classes.jar" "${WORK_DIR}/"

mkdir -p "${WORK_DIR}/lib"
if [ -d "${WORK_DIR}/aar_contents/jni" ]; then
    cp -r "${WORK_DIR}/aar_contents/jni/"* "${WORK_DIR}/lib/"
fi

# 2. Generar AndroidManifest.xml
cat << 'MANIFEST' > "${WORK_DIR}/AndroidManifest.xml"
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.egonzalez.gosms"
    android:versionCode="1"
    android:versionName="1.0">

    <uses-sdk android:minSdkVersion="16" android:targetSdkVersion="33" />
    <uses-feature android:glEsVersion="0x00020000" android:required="true" />
    <uses-permission android:name="android.permission.INTERNET" />

    <application
        android:label="GoSMS"
        android:hasCode="true"
        android:theme="@android:style/Theme.NoTitleBar.Fullscreen">
        <activity
            android:name="com.egonzalez.gosms.MainActivity"
            android:configChanges="keyboardHidden|orientation|screenSize"
            android:exported="true"
            android:screenOrientation="sensorLandscape"
            android:launchMode="singleTop">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
MANIFEST

# 3. Generar MainActivity.java
cat << 'JAVA' > "${WORK_DIR}/java/com/egonzalez/gosms/MainActivity.java"
package com.egonzalez.gosms;

import android.app.Activity;
import android.os.Bundle;
import android.view.WindowManager;
import com.egonzalez.gosms.mobile.EbitenView;

public class MainActivity extends Activity {
    private EbitenView ebitenView;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        ebitenView = new EbitenView(this);
        setContentView(ebitenView);
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (ebitenView != null) {
            ebitenView.suspendGame();
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (ebitenView != null) {
            ebitenView.resumeGame();
        }
    }
}
JAVA

# 4. Compilar MainActivity.java
javac -source 1.8 -target 1.8 \
    -bootclasspath "${PLATFORM_JAR}" \
    -cp "${WORK_DIR}/classes.jar" \
    -d "${WORK_DIR}/classes_compiled" \
    "${WORK_DIR}/java/com/egonzalez/gosms/MainActivity.java"

# 5. Generar DEX
"${BUILD_TOOLS}/d8" \
    --lib "${PLATFORM_JAR}" \
    --output "${WORK_DIR}/dex" \
    "${WORK_DIR}/classes.jar" \
    $(find "${WORK_DIR}/classes_compiled" -name "*.class")

# 6. Enlazar APK base
"${BUILD_TOOLS}/aapt2" link \
    -I "${PLATFORM_JAR}" \
    --manifest "${WORK_DIR}/AndroidManifest.xml" \
    -o "${WORK_DIR}/base_unaligned.apk"

# 7. Empaquetar
cd "${WORK_DIR}"
cp dex/classes.dex .
zip -u base_unaligned.apk classes.dex > /dev/null
zip -ur base_unaligned.apk lib > /dev/null
cd /app

# 8. Zipalign
"${BUILD_TOOLS}/zipalign" -f -p 4 "${WORK_DIR}/base_unaligned.apk" "${FINAL_APK}"

# 9. Firmar
KEYSTORE="${WORK_DIR}/debug.keystore"
if [ ! -f "${KEYSTORE}" ]; then
    keytool -genkeypair -validity 10000 -dname "CN=gosms, O=gosms, C=ES" \
        -keystore "${KEYSTORE}" -storepass android -keypass android \
        -alias androiddebugkey -keyalg RSA -keysize 2048 2> /dev/null
fi

"${BUILD_TOOLS}/apksigner" sign \
    --ks "${KEYSTORE}" \
    --ks-pass pass:android \
    --ks-key-alias androiddebugkey \
    "${FINAL_APK}"

chmod 666 "${FINAL_APK}"
chmod 666 "${AAR_PATH}" 2>/dev/null || true
