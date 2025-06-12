// server.js
// Carga las variables de entorno desde el archivo .env
require('dotenv').config();
const express = require('express');
const { google } = require('googleapis');
const path = require('path');
const nodemailer = require('nodemailer');
// const bcrypt = require('bcryptjs'); // Descomentar si decides usar hashing de contraseñas

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware para JSON y CORS
app.use(express.json());
app.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  next();
});

// Sirve archivos estáticos (HTML, CSS, JS, imágenes) desde la carpeta 'public'
app.use(express.static(path.join(__dirname, 'public')));

// Configuración de Google Sheets
const SPREADSHEET_ID = '1I6pVLSBav-U7c86FLavh0tikPghLDVrWCFuru-qwQ4Y'; // Tu ID de Spreadsheet

const serviceAccount = JSON.parse(process.env.GOOGLE_SERVICE_ACCOUNT);
const auth = new google.auth.GoogleAuth({
  credentials: serviceAccount,
  scopes: ['https://www.googleapis.com/auth/spreadsheets']
});

let sheets;
async function initializeGoogleSheetsClient() {
  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });
    console.log('Google Sheets client initialized.');
  } catch (error) {
    console.error('Error initializing Google Sheets client:', error);
  }
}

initializeGoogleSheetsClient();

// Helper para calcular el nivel
function calculateLevel(exp) {
  if (exp >= 1000) return 5;
  if (exp >= 500) return 4;
  if (exp >= 200) return 3;
  if (exp >= 50) return 2;
  return 1;
}

// Ruta para obtener datos del estudiante por ID
app.get('/student/:id', async (req, res) => {
  const studentId = req.params.id;
  const { career } = req.query; // Obtener la carrera de los parámetros de consulta

  if (!career) {
    return res.status(400).json({ error: 'Career parameter is required.' });
  }

  const sheetName = career === 'Com' ? 'Com' : 'Sheet1'; // Determina el nombre de la hoja

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:L`, // Updated range to include column L
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No data found.' });
    }

    // Buscar el estudiante por ID (columna A)
    const studentRow = rows.find(row => row[0] === studentId);

    if (!studentRow) {
      return res.status(404).json({ error: 'Student not found.' });
    }

    const [id, nombre, email, password, tareas, monedas, asistencias, lastLogin, lastLoginDate, exp, lastLoginLevel, gamesChallenges] = studentRow; // Added gamesChallenges

    res.json({
      id,
      nombre,
      email,
      password,
      tareas: parseInt(tareas) || 0,
      monedas: parseInt(monedas) || 0,
      asistencias: parseInt(asistencias) || 0,
      lastLogin,
      lastLoginDate,
      exp: parseInt(exp) || 0,
      lastLoginLevel: parseInt(lastLoginLevel) || 1,
      level: calculateLevel(parseInt(exp) || 0),
      gamesChallenges: gamesChallenges || '', // Added gamesChallenges
    });
  } catch (error) {
    console.error('Error fetching student data:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});

// Ruta para obtener datos de todos los estudiantes (para el administrador)
app.get('/admin/students', async (req, res) => {
  const { career } = req.query;

  if (!career) {
    return res.status(400).json({ error: 'Career parameter is required.' });
  }

  const sheetName = career === 'Com' ? 'Com' : 'Sheet1';

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:L`, // Updated range to include column L
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No data found.' });
    }

    // Asume que la primera fila son los encabezados
    const headers = rows[0];
    const students = rows.slice(1).map(row => {
      const [id, nombre, email, password, tareas, monedas, asistencias, lastLogin, lastLoginDate, exp, lastLoginLevel, gamesChallenges] = row; // Added gamesChallenges
      return {
        id,
        nombre,
        email,
        password,
        tareas: parseInt(tareas) || 0,
        monedas: parseInt(monedas) || 0,
        asistencias: parseInt(asistencias) || 0,
        lastLogin,
        lastLoginDate,
        exp: parseInt(exp) || 0,
        lastLoginLevel: parseInt(lastLoginLevel) || 1,
        level: calculateLevel(parseInt(exp) || 0),
        gamesChallenges: gamesChallenges || '', // Added gamesChallenges
      };
    });
    res.json(students);
  } catch (error) {
    console.error('Error fetching admin student data:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});

// Ruta para actualizar los datos de un estudiante
app.put('/student/:id', async (req, res) => {
  const studentId = req.params.id;
  const { tareas, monedas, asistencias, exp, career, gamesChallenges } = req.body; // Added gamesChallenges

  if (!career) {
    return res.status(400).json({ error: 'Career parameter is required.' });
  }

  const sheetName = career === 'Com' ? 'Com' : 'Sheet1';

  try {
    // Obtener todas las filas para encontrar el índice del estudiante
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:L`, // Updated range to include column L
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No data found in sheet.' });
    }

    const rowIndex = rows.findIndex(row => row[0] === studentId); // Encuentra la fila del estudiante por ID

    if (rowIndex === -1) {
      return res.status(404).json({ error: 'Student not found for update.' });
    }

    // Las filas en Google Sheets son 1-indexadas, por lo que sumamos 1
    const actualRowIndex = rowIndex + 1;
    const studentRow = rows[rowIndex];

    const currentLastLoginLevel = parseInt(studentRow[10]) || 0; // Columna K (índice 10)
    const newLevel = calculateLevel(exp);

    const updates = [
      { range: `${sheetName}!F${actualRowIndex}`, values: [[tareas]] }, // Tareas (F)
      { range: `${sheetName}!G${actualRowIndex}`, values: [[monedas]] }, // Monedas (G)
      { range: `${sheetName}!H${actualRowIndex}`, values: [[asistencias]] }, // Asistencias (H)
      { range: `${sheetName}!J${actualRowIndex}`, values: [[exp]] }, // EXP (J)
      { range: `${sheetName}!L${actualRowIndex}`, values: [[gamesChallenges]] } // Games / Challenges (L)
    ];

    // Only update LAST_LOGIN_LEVEL if the calculated level changes
    if (newLevel !== currentLastLoginLevel) {
      updates.push({
        range: `${sheetName}!K${actualRowIndex}`, // LAST_LOGIN_LEVEL (K)
        values: [[newLevel]]
      });
    }

    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: updates,
        valueInputOption: 'RAW'
      }
    });

    res.json({ success: true, message: `Student ${studentId} data updated successfully.`, newLevel: newLevel });
  } catch (error) {
    console.error('Error updating student data:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});


// Ruta para enviar correo electrónico de recuperación de contraseña
app.post('/send-reset-code', async (req, res) => {
  const { email, career } = req.body;

  if (!career) {
    return res.status(400).json({ error: 'Career parameter is required.' });
  }

  const sheetName = career === 'Com' ? 'Com' : 'Sheet1';

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:D`, // ID (A) y Email (C)
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No data found.' });
    }

    const studentRow = rows.find(row => row[2] === email); // Buscar por email en columna C

    if (!studentRow) {
      return res.status(404).json({ error: 'Email not found.' });
    }

    const studentId = studentRow[0]; // Obtener el ID del estudiante (columna A)
    const resetCode = Math.floor(100000 + Math.random() * 900000).toString(); // Código de 6 dígitos

    // Actualizar la hoja de cálculo con el código de reinicio (columna N)
    const rowIndex = rows.findIndex(row => row[2] === email) + 1; // +1 porque las filas de Sheets son 1-indexadas
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!N${rowIndex}`, // Columna N para el código de reinicio
      valueInputOption: 'RAW',
      requestBody: {
        values: [[resetCode]],
      },
    });

    // Configuración de Nodemailer
    const transporter = nodemailer.createTransport({
      service: 'gmail',
      auth: {
        user: process.env.EMAIL_USER,
        pass: process.env.EMAIL_PASS,
      },
    });

    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: email,
      subject: 'Código de recuperación de contraseña',
      text: `Tu código de recuperación es: ${resetCode}. Este código es válido por 10 minutos.`,
    };

    await transporter.sendMail(mailOptions);
    res.json({ success: true, message: 'Código de recuperación enviado.', resetId: studentId });
  } catch (error) {
    console.error('Error sending reset code:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});

// Ruta para verificar el código de reinicio
app.post('/verify-reset-code', async (req, res) => {
  const { resetId, resetCode, career } = req.body;

  if (!career) {
    return res.status(400).json({ error: 'Career parameter is required.' });
  }

  const sheetName = career === 'Com' ? 'Com' : 'Sheet1';

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:N`, // ID (A) y Código de Reinicio (N)
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No data found.' });
    }

    const studentRow = rows.find(row => row[0] === resetId); // Buscar por ID en columna A

    if (!studentRow) {
      return res.status(404).json({ error: 'Student not found.' });
    }

    const storedResetCode = studentRow[13]; // Código de reinicio en columna N (índice 13)

    if (storedResetCode === resetCode) {
      // Opcional: limpiar el código de reinicio después de la verificación exitosa
      const rowIndex = rows.findIndex(row => row[0] === resetId) + 1;
      await sheets.spreadsheets.values.update({
        spreadsheetId: SPREADSHEET_ID,
        range: `${sheetName}!N${rowIndex}`,
        valueInputOption: 'RAW',
        requestBody: {
          values: [['']], // Limpiar el código
        },
      });
      res.json({ success: true, message: 'Código verificado.' });
    } else {
      res.status(400).json({ error: 'Código inválido.' });
    }
  } catch (error) {
    console.error('Error verifying reset code:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});

// Ruta para actualizar la contraseña
app.post('/update-password', async (req, res) => {
  const { resetId, newPassword, career } = req.body;

  if (!career) {
    return res.status(400).json({ error: 'Career parameter is required.' });
  }

  const sheetName = career === 'Com' ? 'Com' : 'Sheet1';

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:D`, // ID (A) y Contraseña (D)
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No data found.' });
    }

    const rowIndex = rows.findIndex(row => row[0] === resetId); // Buscar por ID en columna A

    if (rowIndex === -1) {
      return res.status(404).json({ error: 'Student not found for password update.' });
    }

    // Actualizar la contraseña (columna D)
    const actualRowIndex = rowIndex + 1;
    // const hashedPassword = await bcrypt.hash(newPassword, 10); // Si usas bcrypt
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!D${actualRowIndex}`, // Columna D para la contraseña
      valueInputOption: 'RAW',
      requestBody: {
        values: [[newPassword]], // Usar newPassword directamente si no hay hashing
      },
    });

    res.json({ success: true, message: 'Contraseña actualizada correctamente.' });
  } catch (error) {
    console.error('Error updating password:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});

// Ruta de inicio de sesión
app.post('/login', async (req, res) => {
  const { id, password, career } = req.body;

  if (!career) {
    return res.status(400).json({ error: 'Career parameter is required.' });
  }

  const sheetName = career === 'Com' ? 'Com' : 'Sheet1';

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:L`, // Updated range to include column L
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No data found.' });
    }

    const studentRow = rows.find(row => row[0] === id);

    if (!studentRow) {
      return res.status(401).json({ error: 'ID o contraseña incorrectos.' });
    }

    // const isMatch = await bcrypt.compare(password, studentRow[3]); // Si usas bcrypt
    const isMatch = (password === studentRow[3]); // Comparación directa sin bcrypt

    if (!isMatch) {
      return res.status(401).json({ error: 'ID o contraseña incorrectos.' });
    }

    // Actualizar la última fecha de inicio de sesión (columna I) y la última hora de inicio de sesión (columna J)
    const rowIndex = rows.findIndex(row => row[0] === id) + 1; // +1 porque las filas de Sheets son 1-indexadas
    const now = new Date();
    const lastLoginDate = now.toLocaleDateString('es-ES');
    const lastLoginTime = now.toLocaleTimeString('es-ES');

    // Aquí se corrige la columna de la última fecha de inicio de sesión
    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: [
          { range: `${sheetName}!I${rowIndex}`, values: [[lastLoginTime]] }, // Última hora de inicio de sesión (I)
          { range: `${sheetName}!J${rowIndex}`, values: [[lastLoginDate]] } // Última fecha de inicio de sesión (J)
        ],
        valueInputOption: 'RAW'
      }
    });

    const [studentId, nombre, email, , tareas, monedas, asistencias, , , exp, lastLoginLevel, gamesChallenges] = studentRow; // Destructure to get gamesChallenges

    res.json({
      success: true,
      message: 'Inicio de sesión exitoso.',
      student: {
        id: studentId,
        nombre,
        email,
        tareas: parseInt(tareas) || 0,
        monedas: parseInt(monedas) || 0,
        asistencias: parseInt(asistencias) || 0,
        exp: parseInt(exp) || 0,
        level: calculateLevel(parseInt(exp) || 0),
        lastLoginLevel: parseInt(lastLoginLevel) || 1,
        gamesChallenges: gamesChallenges || '', // Include gamesChallenges in the response
      }
    });
  } catch (error) {
    console.error('Error en el inicio de sesión:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});


// Ruta para el inicio de sesión de administrador
app.post('/admin/login', async (req, res) => {
  const { username, password } = req.body;
  const adminSheetName = 'Admin'; // Suponiendo que los administradores están en una hoja llamada 'Admin'

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${adminSheetName}!A:B`, // Username (A), Password (B)
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No admin data found.' });
    }

    const adminUser = rows.find(row => row[0] === username);

    if (!adminUser) {
      return res.status(401).json({ error: 'Usuario o contraseña de administrador incorrectos.' });
    }

    // const isMatch = await bcrypt.compare(password, adminUser[1]); // Si usas bcrypt
    const isMatch = (password === adminUser[1]); // Comparación directa sin bcrypt

    if (!isMatch) {
      return res.status(401).json({ error: 'Usuario o contraseña de administrador incorrectos.' });
    }

    res.json({ success: true, message: 'Inicio de sesión de administrador exitoso.' });
  } catch (error) {
    console.error('Error en el inicio de sesión de administrador:', error);
    res.status(500).json({ error: 'Internal server error.' });
  }
});


// Iniciar el servidor
app.listen(PORT, () => {
  console.log(`Server running on port ${PORT}`);
});