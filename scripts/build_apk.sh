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

    <uses-sdk android:minSdkVersion="21" android:targetSdkVersion="33" />
    <uses-feature android:glEsVersion="0x00020000" android:required="true" />
    <uses-permission android:name="android.permission.INTERNET" />

    <application
        android:label="GoSMS"
        android:hasCode="true"
        android:hardwareAccelerated="true"
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
import android.content.Intent;
import android.content.SharedPreferences;
import android.database.Cursor;
import android.net.Uri;
import android.os.Bundle;
import android.provider.OpenableColumns;
import android.view.View;
import android.view.WindowManager;

import com.egonzalez.gosms.mobile.EbitenView;
import com.egonzalez.gosms.mobile.Mobile;
import com.egonzalez.gosms.mobile.Picker;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;

public class MainActivity extends Activity implements Picker {
    private static final int PICK_ROM = 1001;
    private static final String PREFS = "gosms";
    private EbitenView ebitenView;
    private SharedPreferences prefs;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
        getWindow().getDecorView().setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY |
                View.SYSTEM_UI_FLAG_FULLSCREEN |
                View.SYSTEM_UI_FLAG_HIDE_NAVIGATION |
                View.SYSTEM_UI_FLAG_LAYOUT_STABLE);

        prefs = getSharedPreferences(PREFS, MODE_PRIVATE);
        ebitenView = new EbitenView(this);
        setContentView(ebitenView);
        Mobile.setPicker(this);
        restoreLastRom();
    }

    // Llamado desde Go cuando el lanzador pide una ROM.
    @Override
    public void pickROM() {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
                intent.addCategory(Intent.CATEGORY_OPENABLE);
                intent.setType("*/*");
                intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION |
                                Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
                startActivityForResult(intent, PICK_ROM);
            }
        });
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != PICK_ROM || resultCode != RESULT_OK || data == null) {
            return;
        }
        final Uri uri = data.getData();
        if (uri == null) {
            return;
        }
        try {
            getContentResolver().takePersistableUriPermission(uri,
                    Intent.FLAG_GRANT_READ_URI_PERMISSION);
        } catch (Exception ignored) {
        }
        final String name = displayName(uri);
        prefs.edit().putString("rom_uri", uri.toString())
                    .putString("rom_name", name).apply();

        new Thread(new Runnable() {
            @Override
            public void run() {
                byte[] bytes = readAll(uri);
                if (bytes != null && bytes.length > 1024) {
                    Mobile.setROM(name, bytes);
                    Mobile.play();
                }
            }
        }).start();
    }

    private void restoreLastRom() {
        final String uriString = prefs.getString("rom_uri", null);
        final String name = prefs.getString("rom_name", "ROM");
        if (uriString == null) {
            return;
        }
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    byte[] bytes = readAll(Uri.parse(uriString));
                    if (bytes != null && bytes.length > 1024) {
                        Mobile.setROM(name, bytes);
                    } else {
                        clearLastRom();
                    }
                } catch (Exception e) {
                    clearLastRom();
                }
            }
        }).start();
    }

    private void clearLastRom() {
        prefs.edit().remove("rom_uri").remove("rom_name").apply();
    }

    private byte[] readAll(Uri uri) {
        InputStream in = null;
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        try {
            in = getContentResolver().openInputStream(uri);
            if (in == null) {
                return null;
            }
            byte[] buffer = new byte[64 * 1024];
            int read;
            while ((read = in.read(buffer)) > 0) {
                out.write(buffer, 0, read);
                if (out.size() > 32 * 1024 * 1024) {
                    return null;
                }
            }
            return out.toByteArray();
        } catch (Exception e) {
            return null;
        } finally {
            try {
                if (in != null) {
                    in.close();
                }
            } catch (Exception ignored) {
            }
        }
    }

    private String displayName(Uri uri) {
        String name = null;
        Cursor cursor = null;
        try {
            cursor = getContentResolver().query(uri, null, null, null, null);
            if (cursor != null && cursor.moveToFirst()) {
                int index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                if (index >= 0) {
                    name = cursor.getString(index);
                }
            }
        } catch (Exception ignored) {
        } finally {
            if (cursor != null) {
                cursor.close();
            }
        }
        if (name == null || name.length() == 0) {
            name = uri.getLastPathSegment();
        }
        return name == null ? "ROM" : name;
    }

    @Override
    public void onBackPressed() {
        // Desde el juego vuelve al lanzador; desde el lanzador sale.
        if (Mobile.handleBack()) {
            super.onBackPressed();
        }
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
        getWindow().getDecorView().setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY |
                View.SYSTEM_UI_FLAG_FULLSCREEN |
                View.SYSTEM_UI_FLAG_HIDE_NAVIGATION |
                View.SYSTEM_UI_FLAG_LAYOUT_STABLE);
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
