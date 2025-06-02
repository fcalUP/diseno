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
// Renombrado de 'points' a 'exp' para consistencia semántica
function calculateLevel(exp) {
  if (exp >= 100) return 5;
  if (exp >= 75) return 4;
  if (exp >= 50) return 3;
  if (exp >= 25) return 2;
  if (exp >= 10) return 1;
  return 0;
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
  const { studentId, password } = req.body;

  if (!studentId || !password) {
    return res.status(400).json({ error: 'ID y contraseña requeridos.' });
  }

  try {
    // Rango ajustado para incluir todas las columnas relevantes hasta la EXP. (Columna J)
    const range = 'Sheet1!A:J';
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
    // Mapear datos del alumno a un objeto más legible usando los índices correctos según tu Sheet
    const id = studentData[0];
    const name = studentData[1] || 'N/A';
    const sexo = studentData[2] || 'N/A';
    const email = studentData[3] || ''; // Columna D: CORREO
    const homeworks = studentData[5] || '0'; // Columna F: Homeworks
    const monedas = studentData[6] || '0'; // Columna G: Coins
    const asistencias = studentData[7] || '0'; // Columna H: Attendance
    // Columna I (Badges) en Sheet1 no se usa directamente para compras, sino la hoja 'Purchases'
    const exp = parseInt(studentData[9]) || 0; // Columna J: EXP
    const level = calculateLevel(exp); // Nivel calculado

    // Obtener las compras de la hoja 'Purchases'
    const purchasesResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A:D'
    });

    const purchaseRows = purchasesResponse.data.values;
    // Filtrar las compras para este studentId
    const studentPurchases = purchaseRows?.length > 1
      ? purchaseRows.slice(1).filter(row => row[1] === id) // row[1] es studentId en Purchases
      : [];

    // Reducir las compras a un mapa de insignias y cantidades
    const badgesMap = studentPurchases.reduce((acc, row) => {
      const badgeName = row[2]; // Columna C es el nombre de la insignia en Purchases
      const quantity = parseInt(row[3]) || 0; // Columna D es la cantidad en Purchases
      acc[badgeName] = (acc[badgeName] || 0) + quantity;
      return acc;
    }, {});

    res.json({
      success: true,
      student: {
        id: id,
        name: name,
        sexo: sexo,
        email: email, // Incluye el email
        monedas: monedas,
        tareas: homeworks, // 'tareas' ahora mapea a 'Homeworks'
        asistencias: asistencias,
        exp: exp, // Renombrado de 'points' a 'exp'
        level: level,
        purchases: badgesMap, // Usa el mapa de compras reales
        rowIndex: studentRowIndex + 1 // Fila en el Sheets (para futuras actualizaciones)
      }
    });

  } catch (error) {
    console.error('Error en el login:', error);
    res.status(500).json({ error: 'Error interno del servidor durante el login.', details: error.message });
  }
});

// Endpoint de Registro
app.post('/api/register', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });

  const { id, name, sexo, password } = req.body;
  if (!id || !name || !sexo || !password || !/^[A-Z0-9]{4}$/.test(password)) {
    return res.status(400).json({ error: 'Datos inválidos o incompletos. Contraseña debe ser 4 HEX mayúsculas.' });
  }

  try {
    const range = 'Sheet1!A:A';
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const existingIds = response.data.values ? response.data.values.flat() : [];

    if (existingIds.includes(id)) {
      return res.status(409).json({ error: 'El ID de alumno ya existe.' });
    }

    // Hash de la contraseña (descomentar y usar 'await bcrypt.hash(password, 10);' si lo reintroduces)
    const storedPassword = password; // Almacenando como texto plano según tu captura de Sheets

    const newRow = [
      id,                  // Columna A: ID
      name,                // Columna B: NOMBRE
      sexo,                // Columna C: Sexo
      '',                  // Columna D: CORREO (vacío por defecto)
      storedPassword,      // Columna E: PAS (Contraseña)
      '0',                 // Columna F: Homeworks (0 por defecto)
      '0',                 // Columna G: Coins (0 por defecto)
      '0',                 // Columna H: Attendance (0 por defecto)
      '0',                 // Columna I: Badges (inicializado con '0' según tu indicación)
      '0'                  // Columna J: EXP (0 por defecto)
    ];

    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Sheet1!A1', // Se agrega al final de la hoja
      valueInputOption: 'RAW',
      requestBody: { values: [newRow] }
    });

    res.status(201).json({ message: 'Alumno registrado exitosamente.' });
  } catch (error) {
    console.error('Error en el registro:', error);
    res.status(500).json({ error: 'Error interno del servidor durante el registro.', details: error.message });
  }
});


// Endpoint para enviar código de restablecimiento
app.post('/api/send-reset-code', async (req, res) => {
  const { id } = req.body;
  if (!id) {
    return res.status(400).json({ error: 'ID de alumno es requerido.' });
  }

  try {
    // Rango para obtener ID y Email (Columna D)
    const range = 'Sheet1!A:D';
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const studentRow = rows.find((row, i) => i > 0 && row[0] === id); // Buscar por ID
    if (!studentRow) {
      return res.status(404).json({ error: 'ID de alumno no encontrado.' });
    }

    const studentEmail = studentRow[3]; // Columna D para el correo electrónico
    if (!studentEmail || studentEmail.trim() === '') {
      return res.status(400).json({ error: 'Este alumno no tiene un correo electrónico registrado para restablecer la contraseña.' });
    }

    const code = Math.floor(100000 + Math.random() * 900000).toString(); // Código de 6 dígitos
    resetCodes[id] = code; // Almacena el código temporalmente

    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: studentEmail,
      subject: 'Código de restablecimiento de contraseña',
      text: `Tu código de restablecimiento de contraseña es: ${code}\nEste código expirará en 10 minutos.`
    };

    await transporter.sendMail(mailOptions);
    console.log(`Código de restablecimiento enviado a ${studentEmail} para ID ${id}`);

    // Limpia el código después de 10 minutos
    setTimeout(() => {
      delete resetCodes[id];
      console.log(`Código de restablecimiento para ID ${id} expirado.`);
    }, 10 * 60 * 1000); // 10 minutos

    res.json({ message: 'Código de restablecimiento enviado a tu correo electrónico.' });
  } catch (error) {
    console.error('Error al enviar el código de restablecimiento:', error);
    res.status(500).json({ error: 'Error al enviar el código de restablecimiento.', details: error.message });
  }
});

// Endpoint para restablecer contraseña con código
app.post('/api/reset-password-with-code', async (req, res) => {
  const { id, code, password } = req.body;
  console.log({ id, code, realCode: resetCodes[id] });

  // Validación de la nueva contraseña (4 HEX mayúsculas)
  if (!id || !code || !password || !/^[A-Z0-9]{4}$/.test(password)) {
    return res.status(400).json({ error: 'Datos inválidos. Asegúrate de usar un código y contraseña válidos (4 HEX mayúsculas).' });
  }

  try {
    if (!resetCodes[id] || resetCodes[id] !== code) {
      return res.status(401).json({ error: 'Código incorrecto o no solicitado.' });
    }

    const range = 'Sheet1!A:A'; // Solo necesitamos la columna A para encontrar la fila
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const index = rows.findIndex((row, i) => i > 0 && row[0] === id); // Buscar por ID
    if (index === -1) {
      return res.status(404).json({ error: 'ID no encontrado.' });
    }

    const rowNumber = index + 1; // Ajustar a número de fila de Sheets
    // const hashedPassword = await bcrypt.hash(password, 10); // Descomentar si usas bcrypt
    const newPassword = password; // Almacenando como texto plano

    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `Sheet1!E${rowNumber}`, // Columna E para la contraseña (PAS)
      valueInputOption: 'RAW',
      requestBody: { values: [[newPassword]] }
    });

    delete resetCodes[id]; // Eliminar el código después de usarlo
    res.json({ message: 'Contraseña actualizada correctamente.' });
  } catch (error) {
    console.error('Error al actualizar contraseña:', error);
    res.status(500).json({ error: 'Error al actualizar contraseña.', details: error.message });
  }
});

// Endpoint para obtener insignias
app.get('/api/badges', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  try {
    const range = 'Badges!A:C'; // Suponiendo Insignia, Cantidad, Costo
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length < 2) { // Asume que la primera fila es el encabezado
      return res.status(404).json({ error: 'No se encontraron insignias o formato incorrecto.' });
    }

    const badges = rows.slice(1).map(row => ({ // Ignorar encabezado
      name: row[0],     // Columna A: Nombre de la Insignia
      quantity: parseInt(row[1]) || 0, // Columna B: Cantidad disponible
      cost: parseInt(row[2]) || 0      // Columna C: Costo
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

  const { studentId, studentRowIndex, itemName, quantityToBuy, itemCost } = req.body;
  if (!studentId || !studentRowIndex || !itemName || typeof quantityToBuy !== 'number' || typeof itemCost !== 'number') {
    return res.status(400).json({ error: 'Datos inválidos' });
  }

  const totalCost = quantityToBuy * itemCost;

  try {
    // Rango de studentCoinsRange debe ser la Columna G (Coins)
    const studentCoinsRange = `Sheet1!G${studentRowIndex}`;
    const badgeDataRange = 'Badges!A:C'; // Nombre, Cantidad, Costo de insignias

    const [coinsResponse, badgesResponse] = await Promise.all([
      sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range: studentCoinsRange }),
      sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range: badgeDataRange })
    ]);

    const currentCoins = parseInt(coinsResponse.data.values?.[0]?.[0]) || 0;
    if (currentCoins < totalCost) {
      return res.status(400).json({ error: 'Monedas insuficientes' });
    }

    const badgeRows = badgesResponse.data.values;
    const badgeRowIndex = badgeRows.findIndex((row, i) => i > 0 && row[0] === itemName);
    if (badgeRowIndex === -1) return res.status(404).json({ error: 'Insignia no encontrada' });

    const currentQty = parseInt(badgeRows[badgeRowIndex][1]) || 0; // Cantidad disponible de la insignia
    if (currentQty < quantityToBuy) {
      return res.status(400).json({ error: `Sólo quedan ${currentQty} unidades de ${itemName}` });
    }

    const newCoins = currentCoins - totalCost;
    const newQty = currentQty - quantityToBuy;

    // Actualizar Google Sheet (Monedas del Alumno y Cantidad de Insignia)
    // *** IMPORTANTE: NO SE AFECTA LA COLUMNA DE EXP. (J) AQUÍ ***
    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: [
          { range: studentCoinsRange, values: [[newCoins]] }, // Columna G: Monedas
          { range: `Badges!B${badgeRowIndex + 1}`, values: [[newQty]] } // Columna B de la hoja Badges
        ],
        valueInputOption: 'RAW' // RAW para valores directos
      }
    });

    // Registrar la compra en la hoja 'Purchases'
    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A:D', // Asegúrate de que esta hoja y columnas existan
      valueInputOption: 'USER_ENTERED',
      requestBody: {
        values: [[
          new Date().toLocaleString(),
          studentId,
          itemName,
          quantityToBuy
        ]]
      }
    });

    // Envío de correo electrónico (mantengo esta parte ya que estaba en tu código)
    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: [`${studentId}@up.edu.mx`, 'fcal@up.edu.mx'],
      subject: 'Confirmación de compra de insignia',
      text: `Hola,

Se ha realizado una compra de insignia:

- Alumno: ${studentId}@up.edu.mx
- Insignia: ${itemName}
- Cantidad: ${quantityToBuy}
- Costo total: ${totalCost} monedas
- Monedas anteriores: ${currentCoins}
- Monedas restantes: ${newCoins}

Gracias.`
    };

    await transporter.sendMail(mailOptions);

    res.json({
      success: true,
      newCoins: newCoins,
      newBadgeQuantity: newQty,
      message: `Compra de ${quantityToBuy} ${itemName} realizada. Monedas restantes: ${newCoins}`
    });

  } catch (error) {
    console.error('Error en la compra:', error);
    res.status(500).json({ error: 'Error interno del servidor durante la compra.', details: error.message });
  }
});

// Endpoint para añadir EXP. (¡Mismo endpoint de antes, ajustado a tu columna EXP.)
// Este endpoint es un EJEMPLO de cómo se podría implementar una forma de ganar EXP.
// En un entorno real, necesitarías una forma segura (admin panel, etc.)
// para usarlo.
app.post('/api/add-exp', async (req, res) => {
  const { studentId, expToAdd } = req.body;

  if (!studentId || expToAdd === undefined || isNaN(expToAdd) || expToAdd <= 0) {
    return res.status(400).json({ error: 'ID de alumno y cantidad de EXP. válida son requeridos.' });
  }

  try {
    const range = 'Sheet1!A:J'; // Necesitamos ID y la columna J (EXP.)
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const index = rows.findIndex((row, i) => i > 0 && row[0] === studentId); // Buscar por ID
    if (index === -1) {
      return res.status(404).json({ error: 'ID de alumno no encontrado.' });
    }

    const rowNumber = index + 1;
    const currentExp = parseInt(rows[index][9]) || 0; // Columna J (índice 9) es EXP.
    const newExp = currentExp + expToAdd;

    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `Sheet1!J${rowNumber}`, // Columna J: EXP.
      valueInputOption: 'RAW',
      requestBody: { values: [[newExp]] }
    });

    const newLevel = calculateLevel(newExp);

    res.json({
      success: true,
      message: `EXP. de ${studentId} actualizada. Nueva EXP.: ${newExp}, Nuevo Nivel: ${newLevel}.`,
      newExp: newExp,
      newLevel: newLevel
    });

  } catch (error) {
    console.error('Error al añadir EXP.:', error);
    res.status(500).json({ error: 'Error interno del servidor al añadir EXP.', details: error.message });
  }
});


// ----------------- INICIAR SERVIDOR -----------------
app.listen(PORT, () => {
  console.log(`Servidor backend escuchando en http://localhost:${PORT}`);
});