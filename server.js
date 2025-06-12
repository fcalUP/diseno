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
    console.log('Cliente de Google Sheets inicializado correctamente.');
  } catch (error) {
    console.error('Error al inicializar Google Sheets:', error);
    process.exit(1); // Sale del proceso si no se puede inicializar el cliente
  }
}
initializeGoogleSheetsClient();

// Almacenamiento temporal para códigos de restablecimiento de contraseña
const resetCodes = {};

// Función para calcular el nivel basada en la EXP.
function calculateLevel(exp) {
  if (exp >= 100) return 5;
  if (exp >= 75) return 4;
  if (exp >= 50) return 3;
  if (exp >= 25) return 2;
  if (exp >= 10) return 1;
  return 0;
}

// Helper para obtener el nombre de la hoja según la carrera
function getSheetName(career) {
  return career === 'Diseno de la Comunicacion' ? 'Com' : 'Sheet1';
}

// Configuración de Nodemailer para el envío de correos
const transporter = nodemailer.createTransport({
  service: 'gmail', // Puedes usar otro servicio o configuración SMTP
  auth: {
    user: process.env.EMAIL_USER,
    pass: process.env.EMAIL_PASS,
  },
});

// ===================================
// ENDPOINTS DE API
// ===================================

// Endpoint de Inicio de Sesión
app.post('/api/login', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  const { studentId, password, career } = req.body; // Recibe la carrera

  if (!studentId || !password || !career) {
    return res.status(400).json({ error: 'ID, contraseña y carrera requeridos.' });
  }

  const sheetName = getSheetName(career);

  try {
    // Rango ajustado para incluir hasta la nueva columna L (Games / Challenges)
    const range = `${sheetName}!A:L`; // Ahora leemos hasta la columna L
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No se encontraron datos de alumnos.' });
    }

    // Buscar el alumno por ID y Contraseña. La contraseña está en Columna E (índice 4)
    const studentRowIndex = rows.findIndex((row, index) => index > 0 && row[0] === studentId && row[4] === password);
    if (studentRowIndex === -1) {
      return res.status(401).json({ error: 'Credenciales inválidas.' });
    }

    const studentData = rows[studentRowIndex]; // Datos de la fila encontrada
    const id = studentData[0]; // ID
    const name = studentData[1] || 'N/A'; // NOMBRE
    const sexo = studentData[2] || 'N/A'; // Sexo
    const email = studentData[3] || ''; // CORREO
    const homeworks = studentData[5] || '0'; // Homeworks
    const monedas = studentData[6] || '0'; // Coins
    const asistencias = studentData[7] || '0'; // Attendance
    const badgesString = studentData[8] || '0'; // Badges (como cadena)
    const exp = parseInt(studentData[9]) || 0; // EXP
    const gamesChallenges = parseInt(studentData[11]) || 0; // Games / Challenges (Columna L, índice 11)


    // Obtener lastLoginLevel. Si está vacío o es undefined (primera carga), se inicializa a 0
    let lastLoginLevel = parseInt(studentData[10]);
    if (isNaN(lastLoginLevel)) { // Si no es un número (es decir, está vacío en la hoja)
        lastLoginLevel = 0; // Considera el nivel en el último login como 0 inicialmente
    }

    const currentLevel = calculateLevel(exp); // Nivel actual calculado

    // Determinar si hay una subida de nivel desde el último login
    const levelUpOccurred = currentLevel > lastLoginLevel;

    // Obtener las compras de la hoja 'Purchases' (asume que Purchases es global o por carrera si lo necesitas)
    const purchasesResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A:D' // Mantendremos una sola hoja de Purchases por simplicidad
    });

    const purchaseRows = purchasesResponse.data.values;
    // Filtrar las compras para este studentId
    const studentPurchases = purchaseRows?.length > 1
      ? purchaseRows.slice(1).filter(row => row[1] === id) // row[1] es studentId en Purchases
      : [];

    // Reducir las compras a un mapa de insignias y cantidades
    const totalBadgesCount = studentPurchases.reduce((sum, row) => sum + (parseInt(row[3]) || 0), 0);

const badgesMap = studentPurchases.reduce((acc, row) => {
      const badgeName = row[2]; // Columna C es el nombre de la insignia en Purchases
      const quantity = parseInt(row[3]) || 0; // Columna D es la cantidad en Purchases
      acc[badgeName] = (acc[badgeName] || 0) + quantity;
      return acc;
    }, {});


    
    // Actualizar columna I (total de badges adquiridas)
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!I${studentRowIndex + 1}`,
      valueInputOption: 'RAW',
      requestBody: { values: [[totalBadgesCount.toString()]] }
    });
    

res.json({
      success: true,
      student: {
        id: id,
        name: name,
        sexo: sexo,
        email: email,
        monedas: monedas,
        tareas: homeworks,
        asistencias: asistencias,
        gamesChallenges: gamesChallenges, // NEW
        exp: exp,
        level: currentLevel, // Nivel actual
        purchases: badgesMap, // Usar el mapa de compras reales
        rowIndex: studentRowIndex + 1 // Fila en el Sheets (para futuras actualizaciones)
      },
      // Indicador para el frontend
      levelUpOccurred: levelUpOccurred
    });

    // ¡Importante! Actualizar el LAST_LOGIN_LEVEL en Google Sheets después del login exitoso
    // Esto asegura que la animación no se dispare en el próximo inicio de sesión si no hay cambio de nivel real.
    // Y también inicializa el valor si estaba vacío.
    if (currentLevel !== lastLoginLevel || isNaN(parseInt(studentData[10]))) { // Actualiza si hay cambio de nivel O si estaba vacío
      await sheets.spreadsheets.values.update({
        spreadsheetId: SPREADSHEET_ID,
        range: `${sheetName}!K${studentRowIndex + 1}`, // Columna K para LAST_LOGIN_LEVEL
        valueInputOption: 'RAW',
        requestBody: { values: [[currentLevel]] }
      });
    }

  } catch (error) {
    console.error('Error en el login:', error);
    res.status(500).json({ error: 'Error interno del servidor durante el login.', details: error.message });
  }
});

// Endpoint de Registro
app.post('/api/register', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });

  const { id, name, sexo, password, career } = req.body; // Recibe la carrera
  if (!id || !name || !sexo || !password || !/^[A-Z0-9]{4}$/.test(password) || !career) {
    return res.status(400).json({ error: 'Datos inválidos o incompletos. Contraseña debe ser 4 HEX mayúsculas y carrera requerida.' });
  }

  const sheetName = getSheetName(career);

  try {
    const range = `${sheetName}!A:A`;
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const existingIds = response.data.values ? response.data.values.flat() : [];

    if (existingIds.includes(id)) {
      return res.status(409).json({ error: 'El ID de alumno ya existe.' });
    }

    const storedPassword = password; // Almacenando como texto plano

    const newRow = [
      id,                  // Columna A: ID
      name,                // Columna B: NOMBRE
      sexo,                // Columna C: Sexo
      id+'@up.edu.mx',     // Columna D: CORREO (vacío por defecto)
      storedPassword,      // Columna E: PAS (Contraseña)
      '0',                 // Columna F: Homeworks (0 por defecto)
      '0',                 // Columna G: Coins (0 por defecto)
      '0',                 // Columna H: Attendance (0 por defecto)
      '0',                 // Columna I: Badges (inicializado con '0')
      '0',                 // Columna J: EXP (0 por defecto)
      '0',                 // Columna K: LAST_LOGIN_LEVEL (0 por defecto para nuevos usuarios)
      '0'                  // Columna L: Games / Challenges (0 por defecto para nuevos usuarios)
    ];

    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A1`, // Se agrega al final de la hoja de la carrera seleccionada
      valueInputOption: 'RAW',
      requestBody: { values: [newRow] }
    });

    res.status(201).json({ message: 'Alumno registrado exitosamente.' });
  } catch (error) {
    console.error('Error en el registro:', error);
    res.status(500).json({ error: 'Error interno del servidor durante el registro.', details: error.message });
  }
});


// server.js

// Endpoint para enviar código de restablecimiento
app.post('/api/send-reset-code', async (req, res) => {
  const { id, career } = req.body; // Recibe la carrera
  if (!id || !career) {
    return res.status(400).json({ error: 'ID de alumno y carrera son requeridos.' });
  }

  const sheetName = getSheetName(career);

  try {
    const range = `${sheetName}!A:A`; // Solo necesitas verificar la existencia del ID en la hoja de la carrera
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values || []; // Asegura que rows sea un array incluso si no hay datos

    // Buscar el alumno por ID para confirmar que existe en la hoja (ignorando la fila de encabezado)
    const studentExists = rows.some((row, i) => i > 0 && row[0] === id);
    if (!studentExists) {
      return res.status(404).json({ error: 'ID de alumno no encontrado en la carrera seleccionada.' });
    }

    // === Lógica clave: Construir el correo electrónico con el ID ===
    const studentEmailToUse = `${id}@up.edu.mx`; // Misma lógica que la compra de insignias

    const code = Math.floor(100000 + Math.random() * 900000).toString();
    // Almacena el código con el ID y la carrera para evitar conflictos entre carreras
    resetCodes[`${id}-${career}`] = code;

    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: [studentEmailToUse, 'fcal@up.edu.mx'],
      subject: 'Código de restablecimiento de contraseña',
      text: `Tu código de restablecimiento de contraseña es: ${code}\nEste código expirará en 10 minutos.`
    };

    // === Añadir logs detallados aquí para ver si Nodemailer se ejecuta y qué devuelve ===
    console.log(`Intentando enviar código a: ${studentEmailToUse} para carrera: ${career}`);
    console.log("Opciones de correo (mailOptions):", mailOptions);

    await transporter.sendMail(mailOptions);
    console.log(`Código de restablecimiento enviado a ${studentEmailToUse} para ID ${id} y carrera ${career}`);

    setTimeout(() => {
      delete resetCodes[`${id}-${career}`]; // Eliminar el código usando la clave combinada
      console.log(`Código de restablecimiento para ID ${id} y carrera ${career} expirado.`);
    }, 10 * 60 * 1000);

    // Se eliminó la actualización innecesaria de la hoja de cálculo que causaba el error

    res.json({ message: 'Código de restablecimiento enviado a tu correo electrónico.' });

  } catch (error) {
    // === Este es el lugar CRÍTICO para ver el error si no es de Nodemailer ===
    console.error('Error al enviar el código de restablecimiento (catch general):', error);
    res.status(500).json({ error: 'Error al enviar el código de restablecimiento.', details: error.message });
  }
});

// Endpoint para restablecer contraseña con código
app.post('/api/reset-password-with-code', async (req, res) => {
  const { id, code, password, career } = req.body; // Recibe la carrera
  console.log({ id, code, realCode: resetCodes[`${id}-${career}`], career }); // Log con la clave combinada

  if (!id || !code || !password || !/^[A-Z0-9]{4}$/.test(password) || !career) {
    return res.status(400).json({ error: 'Datos inválidos. Asegúrate de usar un código, contraseña válida (4 HEX mayúsculas) y carrera.' });
  }

  const sheetName = getSheetName(career);

  try {
    if (!resetCodes[`${id}-${career}`] || resetCodes[`${id}-${career}`] !== code) {
      return res.status(401).json({ error: 'Código incorrecto o no solicitado.' });
    }

    const range = `${sheetName}!A:A`; // Solo necesitamos la columna A para encontrar la fila en la hoja correcta
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const index = rows.findIndex((row, i) => i > 0 && row[0] === id); // Buscar por ID
    if (index === -1) {
      return res.status(404).json({ error: 'ID no encontrado en la carrera seleccionada.' });
    }

    const rowNumber = index + 1;
    const newPassword = password; // Almacenando como texto plano

    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!E${rowNumber}`, // Columna E para la contraseña (PAS) en la hoja correcta
      valueInputOption: 'RAW',
      requestBody: { values: [[newPassword]] }
    });

    delete resetCodes[`${id}-${career}`]; // Eliminar el código después de usarlo, usando la clave combinada

res.json({ message: 'Contraseña actualizada correctamente.' });
  } catch (error) {
    console.error('Error al actualizar contraseña:', error);
    res.status(500).json({ error: 'Error al actualizar contraseña.', details: error.message });
  }
});

// Endpoint para obtener insignias
app.get('/api/badges', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  const { career } = req.query; // Recibe la carrera como query parameter

  // Asumiendo que las insignias son las mismas para ambas carreras y se encuentran en la hoja 'Badges'
  // Si las insignias fueran diferentes por carrera, necesitarías otra lógica aquí.
  const badgesSheetName = 'Badges'; // O podrías tener 'Badges_DD' y 'Badges_Com'

  try {
    const range = `${badgesSheetName}!A:C`; // Suponiendo Insignia, Cantidad, Costo
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length < 2) { // Asume que la primera fila es el encabezado
      return res.status(404).json({ error: 'No se encontraron insignias o formato incorrecto.' });
    }

    const badges = rows.slice(1).map(row => ({ // Ignorar encabezado
      name: row[0], // Columna A: Nombre de la Insignia
      quantity: parseInt(row[1]) || 0, // Columna B: Cantidad disponible
      cost: parseInt(row[2]) || 0 // Columna C: Costo
    }));

    res.json({ success: true, badges });
  } catch (error) {
    console.error('Error al obtener insignias:', error);
    res.status(500).json({ error: 'Error al obtener insignias', details: error.message });
  }
});

// Endpoint para realizar una compra
app.post('/api/purchase', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });

  const { studentId, studentRowIndex, itemName, quantityToBuy, itemCost, career } = req.body;

  if (!studentId || !studentRowIndex || !itemName || typeof quantityToBuy !== 'number' || typeof itemCost !== 'number' || !career) {
    return res.status(400).json({ error: 'Datos inválidos o incompletos para la compra.' });
  }

  const sheetName = getSheetName(career);

  try {
    // 1. Obtener datos actuales del alumno para verificar monedas y EXP
    const studentRange = `${sheetName}!G${studentRowIndex}:J${studentRowIndex}`; // Monedas (G), Badges (I), EXP (J)
    const studentResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: studentRange
    });
    const studentRow = studentResponse.data.values?.[0];

    if (!studentRow) {
      return res.status(404).json({ error: 'Student row not found.' });
    }

    let currentMonedas = parseInt(studentRow[0]) || 0; // Monedas (G)
    let currentExp = parseInt(studentRow[3]) || 0; // EXP (J)

    // 2. Obtener datos de la insignia de la hoja 'Badges' para verificar disponibilidad
    const badgesResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Badges!A:C'
    });
    const badgeRows = badgesResponse.data.values;

    const badgeIndex = badgeRows ? badgeRows.findIndex(row => row[0] === itemName) : -1;
    if (badgeIndex === -1) {
      return res.status(404).json({ error: 'Insignia no encontrada.' });
    }

    const availableQuantity = parseInt(badgeRows[badgeIndex][1]) || 0;
    const badgeCost = parseInt(badgeRows[badgeIndex][2]) || 0;

    if (availableQuantity < quantityToBuy) {
      return res.status(400).json({ error: `No hay suficientes unidades de ${itemName} disponibles.` });
    }

    const totalCost = itemCost * quantityToBuy;
    if (currentMonedas < totalCost) {
      return res.status(400).json({ error: 'Monedas insuficientes.' });
    }

    // 3. Realizar la compra
    const newMonedas = currentMonedas - totalCost;
    const expGain = totalCost; // Gana 1 EXP por cada moneda gastada
    const newExp = currentExp + expGain;

    // Actualizar la hoja de Compras
    const purchaseRow = [
      new Date().toISOString(), // Timestamp
      studentId,                  // Student ID
      itemName,                   // Item Name
      quantityToBuy.toString()    // Quantity
    ];
    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A1', // Agrega a la hoja de compras
      valueInputOption: 'RAW',
      requestBody: { values: [purchaseRow] }
    });

    // Actualizar la cantidad disponible de la insignia en la hoja 'Badges'
    const newBadgeQuantity = availableQuantity - quantityToBuy;
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `Badges!B${badgeIndex + 1}`, // Columna B (Cantidad) de la fila de la insignia
      valueInputOption: 'RAW',
      requestBody: { values: [[newBadgeQuantity.toString()]] }
    });

    // Actualizar Monedas y EXP en la hoja del alumno
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!G${studentRowIndex}`, // Monedas (G)
      valueInputOption: 'RAW',
      requestBody: { values: [[newMonedas.toString()]] }
    });
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!J${studentRowIndex}`, // EXP (J)
      valueInputOption: 'RAW',
      requestBody: { values: [[newExp.toString()]] }
    });

    res.json({
      success: true,
      message: 'Compra realizada exitosamente.',
      newCoins: newMonedas.toString(),
      newExp: newExp.toString(),
      newBadgeQuantity: newBadgeQuantity,
    });

  } catch (error) {
    console.error('Error al procesar la compra:', error);
    res.status(500).json({ error: 'Error interno del servidor al procesar la compra.', details: error.message });
  }
});


// Endpoint para obtener todos los estudiantes para el admin
app.get('/api/admin/students', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  const { career } = req.query; // Obtener la carrera desde los query parameters

  if (!career) {
    return res.status(400).json({ error: 'Carrera requerida para obtener datos de estudiantes.' });
  }

  const sheetName = getSheetName(career);

  try {
    // Rango ajustado para incluir hasta la columna L (Games / Challenges)
    const range = `${sheetName}!A:L`; // Leemos hasta la columna L
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: range
    });
    const rows = response.data.values;

    if (!rows || rows.length < 2) { // Considera la primera fila como encabezado
      return res.status(404).json({ error: 'No se encontraron datos de alumnos para la carrera seleccionada.' });
    }

    const students = rows.slice(1).map((row, index) => ({
      id: row[0],
      name: row[1],
      sexo: row[2],
      email: row[3], // Columna D
      password: row[4], // Columna E (Contraseña)
      tareas: parseInt(row[5]) || 0, // Columna F
      monedas: parseInt(row[6]) || 0, // Columna G
      asistencias: parseInt(row[7]) || 0, // Columna H
      badges: parseInt(row[8]) || 0, // Columna I
      exp: parseInt(row[9]) || 0, // Columna J
      lastLoginLevel: parseInt(row[10]) || 0, // Columna K
      gamesChallenges: parseInt(row[11]) || 0, // Columna L
      rowIndex: index + 2 // +2 porque slice(1) y 0-indexed
    }));

    res.json({ success: true, students });
  } catch (error) {
    console.error('Error fetching admin students data:', error);
    res.status(500).json({ error: 'Error interno del servidor al obtener datos de alumnos para el administrador.', details: error.message });
  }
});

// Endpoint para actualizar datos de un estudiante por el admin
app.post('/api/admin/update-student-data', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });

  const { id, career, rowIndex, tareas, monedas, asistencias, exp, gamesChallenges } = req.body; // Add gamesChallenges

  if (!id || !career || !rowIndex) {
    return res.status(400).json({ error: 'ID, carrera y rowIndex son requeridos.' });
  }

  const sheetName = getSheetName(career);

  try {
    // Antes de actualizar, obtén el LAST_LOGIN_LEVEL actual
    const studentResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!K${rowIndex}` // Columna K (índice 10)
    });
    const studentRow = studentResponse.data.values?.[0];

    if (!studentRow) {
      return res.status(404).json({ error: 'Student row not found.' });
    }

    const currentLastLoginLevel = parseInt(studentRow[0]) || 0; // Columna K (índice 0 en el rango de una celda)
    const newLevel = calculateLevel(exp);

    const updates = [
      { range: `${sheetName}!F${rowIndex}`, values: [[tareas]] }, // Tareas (F)
      { range: `${sheetName}!G${rowIndex}`, values: [[monedas]] }, // Monedas (G)
      { range: `${sheetName}!H${rowIndex}`, values: [[asistencias]] }, // Asistencias (H)
      { range: `${sheetName}!J${rowIndex}`, values: [[exp]] }, // EXP (J)
      { range: `${sheetName}!L${rowIndex}`, values: [[gamesChallenges]] } // Games / Challenges (L)
    ];

    // Only update LAST_LOGIN_LEVEL if the calculated level changes
    if (newLevel !== currentLastLoginLevel) {
      updates.push({
        range: `${sheetName}!K${rowIndex}`, // LAST_LOGIN_LEVEL (K)
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

    res.json({ success: true, message: `Student ${id} data updated successfully.`, newLevel: newLevel });
  } catch (error) {
    console.error('Error updating student data:', error);
    res.status(500).json({ error: 'Error interno del servidor al actualizar datos del alumno.', details: error.message });
  }
});


// Endpoint para login del administrador (ejemplo simple, en producción usar bcryptjs)
app.post('/api/admin-login', (req, res) => {
  const { username, password } = req.body;
  if (username === process.env.ADMIN_USERNAME && password === process.env.ADMIN_PASSWORD) {
    res.json({ success: true, message: 'Admin login successful.' });
  } else {
    res.status(401).json({ error: 'Invalid admin credentials.' });
  }
});


// Sirve el archivo HTML principal
app.get('/', (req, res) => {
  res.sendFile(path.join(__dirname, 'public', 'index.html'));
});

// Inicia el servidor
app.listen(PORT, () => {
  console.log(`Servidor iniciado en http://localhost:${PORT}`);
});